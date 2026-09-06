// T260, T295. The generic half of the browser gate: contracts/pages.md's
// seven assertions (status, the four shell landmarks, the page's own landmark
// non-empty, title, zero console/CSP/network problems), run once per page in
// the application's OWN route inventory (e2e/routes.ts) and once for each of
// the two reachable error views, at both viewports (playwright.config.ts's
// two projects).
//
// The list is never hand-kept: a page routes.ts does not enumerate is a page
// e2e/httproute's own boot-time panic already refused to serve (FR-067), and
// a page whose credential or title this file cannot work out fails loudly,
// naming the op id, rather than being quietly skipped (T295 is what a page
// with no case at all looks like: it never reaches this file, and
// gate_test.go on the Go side is what keeps `routes --json` from silently
// stopping to list it).
//
// smoke.spec.ts is unchanged and keeps the page-specific assertions — what
// the record list contains, what the settings danger zone states, and so on.
// This file only ever runs the seven generic ones.
import { fixtures } from "./fixtures";
import { open } from "./gate";
import { credentialFor, pageRoutes, type PageRoute } from "./routes";
import { expect, test } from "./auth";

import type { Page } from "@playwright/test";

async function apiNameOf(page: Page, id: string): Promise<string> {
  const response = await page.request.get(`/api/v1/records/medications/${id}`);
  expect(
    response.ok(),
    `routes.gate: the API did not answer for record ${id}`,
  ).toBe(true);

  return ((await response.json()) as { name: string }).name;
}

async function patientNameOf(page: Page, id: string): Promise<string> {
  const response = await page.request.get(`/api/v1/patients/${id}`);
  expect(
    response.ok(),
    `routes.gate: the API did not answer for patient ${id}`,
  ).toBe(true);

  const patient = (await response.json()) as {
    first_name: string;
    last_name: string;
  };
  return `${patient.first_name} ${patient.last_name}`.trim();
}

async function nameOf(page: Page, url: string): Promise<string> {
  const response = await page.request.get(url);
  expect(response.ok(), `routes.gate: the API did not answer for ${url}`).toBe(
    true,
  );

  return ((await response.json()) as { name: string }).name;
}

// fieldOf is nameOf generalised to a member other than `name`: immunization's
// detail page titles itself after vaccine_name, the same way medication's
// titles itself after name (medicationDetailPage's own apiNameOf above).
async function fieldOf(
  page: Page,
  url: string,
  field: string,
): Promise<string> {
  const response = await page.request.get(url);
  expect(response.ok(), `routes.gate: the API did not answer for ${url}`).toBe(
    true,
  );

  return ((await response.json()) as Record<string, string>)[field];
}

// insuranceCompanyOf: insurance titles its detail page after the insurer
// (the "company" member), not "name" — the one field nameOf's shape assumes.
async function insuranceCompanyOf(page: Page, id: string): Promise<string> {
  const response = await page.request.get(`/api/v1/records/insurance/${id}`);
  expect(
    response.ok(),
    `routes.gate: the API did not answer for insurance ${id}`,
  ).toBe(true);

  return ((await response.json()) as { company: string }).company;
}

// idOf reads the record id P5's SmokeURL is bound to, off the end of the URL
// itself, so nothing here needs a second copy of the kind's URL segment.
function idOf(smokeURL: string): string {
  return smokeURL.slice(smokeURL.lastIndexOf("/") + 1);
}

// titleFor is contracts/pages.md's title column, worked out generically:
// internal/web/page/errors.go titles the error views after their own
// landmark's name plus the product suffix, and seven of the nine pages do the
// same. The two that do not are named here, each with the reason — not
// guessed at, and not silently skipped:
//
//   - P5 (medicationDetailPage) titles itself after the RECORD, never the
//     landmark's fixed name ("Medication"), so its title is read off the same
//     API the page itself renders from.
//   - P9 (verifyEmailPage) is titled "Confirm your address" while its
//     landmark is named "Email confirmation" — contracts/pages.md's own row
//     gives the pair different words.
//   - searchPage's landmark is the bare `search` role (research: the native
//     <search> element carries the role with no accessible name), so there
//     is no name to read the generic rule off; it is titled "Search" here.
//
// Any OTHER page that does not follow the generic rule and has no entry here
// fails assertion 4 with its own op id in the mismatch, which is this file
// doing exactly the job contracts/pages.md's gate exists for.
async function titleFor(route: PageRoute, page: Page): Promise<string> {
  switch (route.opID) {
    case "verifyEmailPage":
      return fixtures.title(fixtures.titles.verifyEmail);
    case "medicationDetailPage":
      return fixtures.title(await apiNameOf(page, idOf(route.smokeURL)));
    case "immunizationDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/immunizations/${idOf(route.smokeURL)}`,
          "vaccine_name",
        ),
      );
    case "injuryDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/injuries/${idOf(route.smokeURL)}`,
          "name",
        ),
      );
    case "insuranceDetailPage":
      return fixtures.title(
        await insuranceCompanyOf(page, idOf(route.smokeURL)),
      );
    case "equipmentDetailPage":
      return fixtures.title(
        await nameOf(page, `/api/v1/records/equipment/${idOf(route.smokeURL)}`),
      );
    case "symptomDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/symptoms/${idOf(route.smokeURL)}`,
          "name",
        ),
      );
    case "measurementsDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/vitals/${idOf(route.smokeURL)}`,
          "recorded_at",
        ),
      );
    case "allergyDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/allergies/${idOf(route.smokeURL)}`,
          "allergen",
        ),
      );
    case "conditionDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/conditions/${idOf(route.smokeURL)}`,
          "diagnosis",
        ),
      );
    case "emergencyContactDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/emergency-contacts/${idOf(route.smokeURL)}`,
          "name",
        ),
      );
    case "encounterDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/encounters/${idOf(route.smokeURL)}`,
          "reason",
        ),
      );
    case "procedureDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/procedures/${idOf(route.smokeURL)}`,
          "name",
        ),
      );
    case "treatmentDetailPage":
      return fixtures.title(
        await fieldOf(
          page,
          `/api/v1/records/treatments/${idOf(route.smokeURL)}`,
          "name",
        ),
      );
    case "familyHistoryDetailPage":
      return fixtures.title(
        await nameOf(
          page,
          `/api/v1/records/family-history/${idOf(route.smokeURL)}`,
        ),
      );
    case "patientListPage":
      return fixtures.title("People");
    case "patientDetailPage":
      return fixtures.title(await patientNameOf(page, idOf(route.smokeURL)));
    case "facilityListPage":
      return fixtures.title("Places of care");
    case "practitionerDetailPage":
      return fixtures.title(
        await nameOf(page, `/api/v1/practitioners/${idOf(route.smokeURL)}`),
      );
    case "facilityDetailPage":
      return fixtures.title(
        await nameOf(page, `/api/v1/facilities/${idOf(route.smokeURL)}`),
      );
    case "searchPage":
      // /search's landmark is the bare `search` role and carries no
      // accessible name of its own (contracts/pages.md P), so the generic
      // "title after the landmark's name" rule below has nothing to read.
      return fixtures.title("Search");
    default:
      return fixtures.title(route.landmark.name ?? "");
  }
}

for (const route of pageRoutes) {
  test.describe(`${route.opID} — the render gate`, () => {
    test.use({ signedInAs: credentialFor(route) });

    test("passes every one of contracts/pages.md's seven assertions", async ({
      page,
    }) => {
      await open(page, {
        path: route.smokeURL,
        title: await titleFor(route, page),
        landmark: route.landmark,
        status: 200,
      });
    });

    // FR-014, ANALYSIS/SHARED-DESIGN §3.0: every authenticated page carries the
    // patient switcher, forever — this reads the inventory's own auth column
    // rather than naming pages, so a phase-003+ page that registers with
    // auth: user inherits the assertion the day it ships, with no edit here.
    if (route.auth === "user") {
      test('carries the "Active patient" switcher (FR-014)', async ({
        page,
      }) => {
        await page.goto(route.smokeURL);
        await expect(
          page.getByRole("combobox", { name: "Active patient" }),
        ).toBeVisible();
      });
    }

    // US9's status-view catalogue: a narrowed URL on the SAME route, so it
    // gets the same seven assertions under this route's own title and
    // landmark, never a second one of its own.
    for (const variant of route.smokeVariants) {
      test(`the smoke variant ${variant} passes the same seven assertions`, async ({
        page,
      }) => {
        await open(page, {
          path: variant,
          title: await titleFor(route, page),
          landmark: route.landmark,
          status: 200,
        });
      });
    }
  });
}

// --- The error views -------------------------------------------------------
//
// E1 (404) and E2 (403) are each reachable by a URL and run through the same
// seven assertions as any page. E3 (500) is not: contracts/pages.md records
// why — no route in a shipped build fails on purpose, so producing one for
// this gate would be a worse defect than an unsmoked error page. It is
// covered instead, at the templ layer, by internal/web/page/errors_test.go
// (T230). Its title, "Something went wrong — MediKube", follows the same
// generic rule as the two below and is asserted there.

test.describe("E1 — not found", () => {
  test.use({ signedInAs: "anonymous" });

  test("a path nothing serves answers 404 inside the full shell (FR-033)", async ({
    page,
  }) => {
    // A sentinel distinct from every path the inventory actually serves,
    // checked here rather than assumed, so a future page cannot collide with
    // it in silence.
    const path = "/routes-gate-404-sentinel-for-smoke";
    expect(
      pageRoutes.some(
        (route) => route.path === path || route.smokeURL === path,
      ),
      "the 404 probe path collides with a real page; pick another sentinel",
    ).toBe(false);

    await open(page, {
      path,
      title: fixtures.title("Not found"),
      landmark: { role: "region", name: "Not found" },
      status: 404,
    });
  });
});

test.describe("E2 — sign in required", () => {
  test.use({ signedInAs: "anonymous" });

  test("a session page opened with no session renders the sign-in prompt, not a 404 (E2)", async ({
    page,
  }) => {
    // Any session-required page probes the same refusal: it is enforced once,
    // at the router (internal/httproute/registry.go's Bind), not per page.
    const sessionPage = pageRoutes.find((route) => route.auth === "user");
    expect(
      sessionPage,
      "routes.ts stopped listing any session-required page to probe E2 with",
    ).toBeTruthy();

    await open(page, {
      path: sessionPage!.smokeURL,
      title: fixtures.title("Sign in required"),
      landmark: { role: "region", name: "Sign in required" },
      status: 403,
    });
  });
});
