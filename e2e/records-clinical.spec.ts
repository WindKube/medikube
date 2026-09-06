// T102. Smoke cases for the two US3 kinds' pages, at both viewports (every
// case here runs once per project in playwright.config.ts). open() carries
// contracts/pages.md's seven assertions; what is asserted here on top of that
// is the one thing only a browser proves: the seeded row is actually on the
// page.
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { open } from "./gate";
import { expect, test } from "./auth";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function goString(relative: string, name: string): string {
  const source = readFileSync(resolve(repositoryRoot, relative), "utf8");
  const found = new RegExp(`\\b${name}\\s*=\\s*"([^"]*)"`).exec(source);
  if (!found) {
    throw new Error(`e2e: ${relative} no longer declares ${name}`);
  }
  return found[1];
}

const fixturesGo = "internal/testsupport/fixtures.go";
const shellGo = "internal/web/views/shell/props.go";

function title(page: string): string {
  return (
    page +
    goString(shellGo, "SuffixSeparator") +
    goString(shellGo, "ProductName")
  );
}

const englishCatalogue = readFileSync(
  resolve(repositoryRoot, "internal/i18n/locales/active.en.toml"),
  "utf8",
);

function catalogueEnglish(id: string): string {
  const escaped = id.replace(/\./g, "\\.");
  const found = new RegExp(`\\[${escaped}\\][^[]*?\\nother = "([^"]*)"`).exec(
    englishCatalogue,
  );
  if (!found) {
    throw new Error(`e2e: active.en.toml declares no ${id}`);
  }
  return found[1];
}

const symptomListTitle = catalogueEnglish("page.symptoms.title");
const vitalsListTitle = catalogueEnglish("page.vitals.title");
const familyMemberListTitle = catalogueEnglish("page.family_history.title");

// Read from internal/testsupport/seed/seed_clinical.go rather than restated:
// the episode's name is what the symptom detail page titles itself with, and
// the measurement set's recorded_at (RFC3339) is what the vitals one does.
const symptomID = goString(fixturesGo, "SymptomHeadacheOneID");
const symptomName = "Headache";
const vitalsID = goString(fixturesGo, "VitalsOneID");
const vitalsRecordedAt = "2025-06-01T07:00:00Z";

const symptomSegment = "symptoms";
const vitalsSegment = "vitals";

// US10's family history: the seeded relative and the patient it is empty on
// live in internal/testsupport/seed/family.go and fixtures.go respectively,
// per contracts/pages.md's instruction to leave /family-history empty on
// account A's self patient (the smoke case for the empty state).
const familyMemberID = goString(
  "internal/testsupport/seed/family.go",
  "FamilyMemberGrandmotherID",
);
const familyMemberName = "Adaeze Okonkwo";
const emptySelfPatientID = goString(fixturesGo, "AccountAPatientSelfID");
const familyMemberSegment = "family-history";

test.describe("the symptom pages", () => {
  test("lists the seeded episode", async ({ page }) => {
    const list = await open(page, {
      path: `/${symptomSegment}`,
      title: title(symptomListTitle),
      landmark: { role: "region", name: "Symptoms" },
    });

    await expect(
      list.locator(`a[href="/${symptomSegment}/${symptomID}"]`),
    ).toBeVisible();
  });

  test("shows one episode", async ({ page }) => {
    await open(page, {
      path: `/${symptomSegment}/${symptomID}`,
      title: title(symptomName),
      landmark: { role: "article", name: "Symptom episode" },
    });
  });
});

test.describe("the measurements pages", () => {
  test("lists the seeded measurement set", async ({ page }) => {
    const list = await open(page, {
      path: `/${vitalsSegment}`,
      title: title(vitalsListTitle),
      landmark: { role: "region", name: "Measurements" },
    });

    await expect(
      list.locator(`a[href="/${vitalsSegment}/${vitalsID}"]`),
    ).toBeVisible();
  });

  test("shows one measurement set", async ({ page }) => {
    await open(page, {
      path: `/${vitalsSegment}/${vitalsID}`,
      title: title(vitalsRecordedAt),
      landmark: { role: "article", name: "Measurement set" },
    });
  });
});

test.describe("the family history pages", () => {
  test("is empty on account A's self patient", async ({ page }) => {
    const list = await open(page, {
      path: `/${familyMemberSegment}?patient=${emptySelfPatientID}`,
      title: title(familyMemberListTitle),
      landmark: { role: "region", name: "Family history" },
    });

    await expect(list.getByText("Nothing recorded yet")).toBeVisible();
  });

  test("shows one relative", async ({ page }) => {
    await open(page, {
      path: `/${familyMemberSegment}/${familyMemberID}`,
      title: title(familyMemberName),
      landmark: { role: "article", name: "Relative" },
    });
  });
});
