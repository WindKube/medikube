// The mail sink the recovery gate reads its link out of (T223p).
//
// A recovery token is a JWT signed from the account's own tokenKey and the
// collection's PasswordResetToken secret, and PocketBase stores no row for it:
// the token exists in the outgoing message and nowhere else. So a browser gate
// that claims to drive the whole flow has to read a real message, which means
// something has to receive one.
//
// It is thirty lines of protocol rather than a mail server. `net/smtp` — which
// is what PocketBase's client is, through mailyak — needs a greeting, EHLO,
// MAIL, RCPT, DATA and QUIT, and nothing else is implemented here: an
// unrecognised verb is answered 502 rather than guessed at. STARTTLS is never
// advertised, so the client never offers to upgrade and there is no
// certificate to invent for a loopback listener.
//
// Captured messages leave over a loopback HTTP endpoint the sink serves
// itself. That endpoint is also the browser gate's readiness signal: it is
// opened only after the instance is up and its SMTP settings point here, so
// "ready" means the whole environment is assembled rather than merely that the
// router answers (e2e/instance.mjs, playwright.config.ts).
//
// Test scaffolding, and unreachable from the production binary by
// construction: it is a Node module under e2e/, so nothing Go compiles can
// import it.
import { createServer as createHttpServer } from 'node:http';
import { createServer as createTcpServer } from 'node:net';

// The two loopback addresses, in the shape and with the defaults
// playwright.config.ts and instance.mjs already use for the instance's own.
export const smtpAddress = process.env.MEDIKUBE_E2E_SMTP_ADDR ?? '127.0.0.1:8025';
export const sinkAddress = process.env.MEDIKUBE_E2E_MAIL_ADDR ?? '127.0.0.1:8026';

// The name the sink answers EHLO with. It is not resolvable and does not need
// to be: no client here verifies it.
const serverName = 'mailsink.medikube.test';

export function splitAddress(value) {
  const separator = value.lastIndexOf(':');
  if (separator < 0) {
    throw new Error(`e2e: ${value} is not a host:port address`);
  }

  return { host: value.slice(0, separator), port: Number(value.slice(separator + 1)) };
}

// A captured message: the envelope the conversation carried and the data the
// client sent, decoded once so a caller reading it does not have to know that
// mailyak writes every part quoted-printable.
function captureOf(envelope, lines) {
  const raw = lines.join('\r\n');

  return {
    from: envelope.from,
    to: envelope.to,
    raw,
    text: decodeQuotedPrintable(raw),
    receivedAt: new Date().toISOString(),
  };
}

// Quoted-printable, which is what a link in one of these messages is wrapped
// in: soft line breaks first — a URL longer than 76 characters is split across
// them, and every recovery link is — then the escaped octets.
function decodeQuotedPrintable(raw) {
  return raw
    .replace(/=\r?\n/g, '')
    .replace(/=([0-9A-Fa-f]{2})/g, (_, hex) => String.fromCharCode(Number.parseInt(hex, 16)));
}

function mailboxOf(line) {
  const found = /<([^>]*)>/.exec(line);

  return found ? found[1] : line.slice(line.indexOf(':') + 1).trim();
}

// converse is the whole protocol. `body` is null between messages and an array
// of lines inside DATA, which is the only state this needs.
function converse(socket, captured) {
  let buffered = '';
  let envelope = { from: '', to: [] };
  let body = null;

  const reply = (line) => socket.write(`${line}\r\n`);

  const handle = (line) => {
    if (body !== null) {
      if (line === '.') {
        captured.push(captureOf(envelope, body));
        body = null;
        envelope = { from: '', to: [] };

        return reply('250 2.0.0 message accepted');
      }

      // Dot-stuffing, undone. A body line that began with a period was sent
      // with a second one so it could not end the message.
      body.push(line.startsWith('..') ? line.slice(1) : line);

      return undefined;
    }

    const verb = line.slice(0, 4).toUpperCase();

    switch (verb) {
      case 'EHLO':
      case 'HELO':
        // One line, so no extension is advertised — STARTTLS least of all.
        return reply(`250 ${serverName}`);
      case 'MAIL':
        envelope.from = mailboxOf(line);

        return reply('250 2.1.0 sender accepted');
      case 'RCPT':
        envelope.to.push(mailboxOf(line));

        return reply('250 2.1.5 recipient accepted');
      case 'DATA':
        body = [];

        return reply('354 end with <CRLF>.<CRLF>');
      case 'RSET':
        envelope = { from: '', to: [] };

        return reply('250 2.0.0 reset');
      case 'NOOP':
        return reply('250 2.0.0 ok');
      case 'QUIT':
        reply('221 2.0.0 bye');

        return socket.end();
      default:
        // Refused rather than guessed at: a verb this sink does not implement
        // is a change in what PocketBase's client sends, and that is worth a
        // failure with the line in it.
        return reply(`502 5.5.2 ${line} is not implemented by the MediKube e2e mail sink`);
    }
  };

  socket.setEncoding('utf8');
  socket.on('error', () => socket.destroy());
  reply(`220 ${serverName} MediKube e2e mail sink`);

  socket.on('data', (chunk) => {
    buffered += chunk;

    for (let end = buffered.indexOf('\r\n'); end >= 0; end = buffered.indexOf('\r\n')) {
      const line = buffered.slice(0, end);
      buffered = buffered.slice(end + 2);
      handle(line);
    }
  });
}

// startMailSink opens the SMTP listener and returns the captured messages plus
// the two remaining controls.
//
// publish() is separate from listen() on purpose: the HTTP endpoint is the
// browser gate's readiness signal, so it must not answer until the caller has
// finished pointing the instance at this sink.
export function startMailSink() {
  const captured = [];
  const smtp = createTcpServer((socket) => converse(socket, captured));
  const address = splitAddress(smtpAddress);

  const http = createHttpServer((request, response) => {
    const url = new URL(request.url ?? '/', `http://${sinkAddress}`);

    if (request.method !== 'GET' || url.pathname !== '/messages') {
      response.writeHead(404, { 'content-type': 'application/json' });

      return response.end(JSON.stringify({ error: 'the mail sink serves GET /messages and nothing else' }));
    }

    const recipient = url.searchParams.get('to');
    const messages = recipient === null ? captured : captured.filter((message) => message.to.includes(recipient));

    response.writeHead(200, { 'content-type': 'application/json' });

    return response.end(JSON.stringify({ messages }));
  });

  return {
    captured,

    listen: () =>
      new Promise((resolve, reject) => {
        smtp.once('error', reject);
        smtp.listen(address.port, address.host, resolve);
      }),

    publish: () =>
      new Promise((resolve, reject) => {
        const where = splitAddress(sinkAddress);
        http.once('error', reject);
        http.listen(where.port, where.host, resolve);
      }),

    close: () => {
      smtp.close();
      http.close();
    },
  };
}
