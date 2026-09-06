#!/usr/bin/env bash
# T028 (specs/007-localisation). FR-005: every application-owned phrase comes
# from the catalogue, never a literal in a screen. This scans every .templ
# source under internal/web/views (never the generated *_templ.go, which is
# markup text by the time it is Go source and would only echo what the source
# already says) for an English-looking literal in one of five shapes: a text
# node, placeholder=, aria-label=, title= or alt=, and an <option> label.
#
# A hit is either fixed (the literal becomes i18n.T/i18n.N) or, when it is
# genuine user-data or a non-text attribute a translator has no business
# touching, allow-listed in scripts/i18n-literals.allow — one line per hit,
# `path:pattern # reason`, checked against the file and pattern that produced
# it so a stale allow-list entry (the literal it names no longer exists) fails
# loudly rather than silently covering for a different, later one.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
targets="$root/internal/web/views"
allow="$root/scripts/i18n-literals.allow"

is_allowed() {
  local file="$1" line="$2"
  [ -f "$allow" ] || return 1
  grep -qF -- "$file:$line:" "$allow"
}

status=0
report() {
  local label="$1" hits="$2"
  [ -z "$hits" ] && return 0

  while IFS= read -r hit; do
    [ -z "$hit" ] && continue
    local file line
    file=$(cut -d: -f1 <<<"$hit")
    line=$(cut -d: -f2 <<<"$hit")
    file=${file#"$root/"}

    if is_allowed "$file" "$line"; then
      continue
    fi

    echo "$label: $file:$line"
    echo "  ${hit#*:*:}"
    status=1
  done <<<"$hits"
}

# A text node: >Word more text< with no {{ (a templ expression) in between.
text_hits=$(grep -rnE --include='*.templ' -- '>[A-Z][a-z]+[^<{]*<' "$targets" 2>/dev/null \
  | grep -v '{{' || true)
report "text node" "$text_hits"

placeholder_hits=$(grep -rnE --include='*.templ' -- 'placeholder="[A-Z]' "$targets" 2>/dev/null || true)
report "placeholder" "$placeholder_hits"

aria_hits=$(grep -rnE --include='*.templ' -- 'aria-label="[A-Z]' "$targets" 2>/dev/null || true)
report "aria-label" "$aria_hits"

title_hits=$(grep -rnE --include='*.templ' -- 'title="[A-Z]' "$targets" 2>/dev/null || true)
report "title" "$title_hits"

alt_hits=$(grep -rnE --include='*.templ' -- 'alt="[A-Z]' "$targets" 2>/dev/null || true)
report "alt" "$alt_hits"

option_hits=$(grep -rnE --include='*.templ' -- '<option[^>]*>[A-Z][a-z]' "$targets" 2>/dev/null \
  | grep -v '{{' || true)
report "option label" "$option_hits"

if [ "$status" -ne 0 ]; then
  echo "task lint:i18n: one or more English literals found above" >&2
  echo "task lint:i18n: fix by extracting through i18n.T/i18n.N, or add a reasoned line to scripts/i18n-literals.allow" >&2
  exit 1
fi

echo "task lint:i18n: clean"
