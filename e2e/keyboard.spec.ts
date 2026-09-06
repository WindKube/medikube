// T215, SC-018. Keyboard-only record -> correct -> tag -> delete, for one
// kind per user story that has shipped on this branch (condition, encounter,
// vitals, immunization, insurance, family member), at both viewports (this
// file runs once per Playwright project, same as every other spec here).
// Every step focuses its control directly rather than tabbing the whole
// document - e2e/a11y.spec.ts already proves every page's controls are
// Tab-reachable in order; this file's own job is the sequence of keyboard
// ACTIONS (typing, Enter, Space) that a11y.spec.ts does not attempt - and a
// visible focus indicator is asserted at every one of them.
//
// The "relate" step is skipped: US6 (record-to-record links) is being
// integrated separately and is not on this branch.
//
// The "tag" step creates one tag on /tags (keyboard-only, same as every
// other control here) and applies it from the record's own edit form via
// tags.Field, the shared picker every kind's form now mounts (US7, FR-064):
// its suggestion button is focused and activated with Enter, the same way
// every other control in this file is, and the applied tag's removable chip
// is asserted both before and after the save round-trip.
//
// A fresh account and a fresh patient per case (mirroring a11y.spec.ts's own
// T163 cases) is what "only operate on records you create in the test" means
// here: the account exists for nothing but this one test, so any row that
// appears after creating is the row this test created. Identifying values
// still carry the project name, per instruction and so a failure's own
// output names which project produced it.
import { randomUUID } from "node:crypto";

import { fixtures } from "./fixtures";
import { expect, test } from "./auth";

import type { Locator, Page } from "@playwright/test";

async function newAccount(page: Page): Promise<void> {
  const email = `keyboard-${randomUUID().slice(0, 8)}@example.test`;
  const registered = await page.request.post(fixtures.registerPath, {
    data: { email, name: "Keyboard Rehearsal", password: fixtures.password },
  });
  expect(registered.status(), await registered.text()).toBe(201);
}

async function newPatient(page: Page): Promise<void> {
  const response = await page.request.post("/api/v1/patients", {
    data: {
      first_name: "Keyboard",
      last_name: "Rehearsal",
      birth_date: "1980-01-01",
      relationship_to_owner: "other",
    },
  });
  expect(
    response.ok(),
    `creating the patient failed: ${await response.text()}`,
  ).toBe(true);
}

// indicatorVisible is keyboard.ts's own check (either outline or box-shadow
// counts), read directly off whatever currently holds focus rather than off
// a walked list, since every step here focuses its own control by name.
async function indicatorVisible(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const element = document.activeElement as HTMLElement | null;
    if (!element || element === document.body) return false;
    const style = getComputedStyle(element);
    return (
      (style.outlineStyle !== "none" && style.outlineWidth !== "0px") ||
      style.boxShadow !== "none"
    );
  });
}

async function focusAndCheck(locator: Locator, label: string): Promise<void> {
  await locator.focus();
  await expect(locator).toBeFocused();
  expect(
    await indicatorVisible(locator.page()),
    `no focus indicator on ${label}`,
  ).toBe(true);
}

async function activate(locator: Locator, label: string): Promise<void> {
  await focusAndCheck(locator, label);
  await locator.page().keyboard.press("Enter");
}

async function typeText(
  locator: Locator,
  label: string,
  value: string,
): Promise<void> {
  await focusAndCheck(locator, label);
  const page = locator.page();
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.insertText(value);
}

// newTag creates one tag on /tags, keyboard-only: Name is the only required
// field, and the manager's own list confirms it landed before this returns,
// so the record-editing step below has a real tag id to pick.
async function newTag(page: Page, name: string): Promise<void> {
  await page.goto("/tags");
  await expect(page).toHaveTitle(fixtures.title("Tags"));

  const manager = page.getByRole("region", { name: "Tags" });
  const createForm = manager.getByRole("form", { name: "Add a tag" });

  await typeText(createForm.getByLabel("Name", { exact: true }), "Name", name);

  const submit = createForm.getByRole("button", { name: "Add tag" });
  await activate(submit, "Add tag");

  await expect(
    manager.getByRole("listitem").filter({ hasText: name }),
  ).toBeVisible();
}

// applyTag drives tags.Field, the shared picker every kind's form mounts
// (US7, FR-064): type the tag's name to narrow the suggestion list to it,
// focus that suggestion directly and activate it with Enter - the same
// shape every other control in this file is driven with - then assert the
// applied tag rendered as its own removable chip.
async function applyTag(form: Locator, name: string): Promise<void> {
  const field = form.getByRole("combobox", { name: "Tags" });
  await typeText(field, "Tags", name);

  const option = form.getByRole("option", { name });
  await activate(option, `tag option "${name}"`);

  await expect(
    form.getByRole("button", { name: `Remove ${name}` }),
  ).toBeVisible();
}

// selectByTyping relies on a native <select>'s own typeahead: typing a
// prefix of an option's visible label selects it, dispatched as real
// keystrokes rather than a programmatic value assignment.
async function selectByTyping(
  locator: Locator,
  label: string,
  optionLabel: string,
): Promise<void> {
  await focusAndCheck(locator, label);
  await locator.page().keyboard.type(optionLabel);
}

// typeDate and typeDateTime type digit-only segments into a native
// date/datetime-local control - the documented way to drive one by keyboard,
// since the segmented widget does not accept a pasted ISO string. Assumed
// en-US locale rendering (month, day, year[, hour, minute, AM/PM]), which is
// what Playwright's own default browser locale renders.
async function typeDate(
  locator: Locator,
  label: string,
  iso: string,
): Promise<void> {
  await focusAndCheck(locator, label);
  const [year, month, day] = iso.split("-");
  await locator.page().keyboard.type(month + day + year);
}

async function typeDateTime(
  locator: Locator,
  label: string,
  iso: string,
): Promise<void> {
  await focusAndCheck(locator, label);
  const [datePart, timePart] = iso.split("T");
  const [year, month, day] = datePart.split("-");
  const [hour, minute] = timePart.split(":");
  // The year segment takes up to six digits and does not advance by itself,
  // so the hour would otherwise land in the year.
  await locator.page().keyboard.type(month + day + year);
  await locator.page().keyboard.press("ArrowRight");
  await locator.page().keyboard.type(hour + minute + "AM");
}

type FieldKind = "text" | "number" | "date" | "datetime" | "select";

type FieldSpec = {
  label: string;
  kind: FieldKind;
  value: string;
};

type KindConfig = {
  label: string;
  segment: string;
  listTitle: string;
  createLabel: string;
  formLabelCreate: string;
  formLabelEdit: string;
  detailLandmark: string;
  // fields is every control filled at creation, in the order the form
  // renders them - order does not have to match Tab order here because each
  // one is located and focused by its own label, not walked to.
  fields: (name: string) => FieldSpec[];
  correctionLabel: string;
  correctionValue: (name: string) => string;
  // headingCheck reads the corrected value back: most kinds show it in the
  // article's own h1; vitals titles itself after a timestamp instead, so its
  // check reads the corrected field's own detail-list entry.
  headingCheck: (article: Locator, corrected: string) => Locator;
};

function scratch(project: string, label: string): string {
  const suffix = project.replace(/[^a-z0-9]/gi, "").slice(0, 16);
  return `E2E ${suffix} ${label}`.slice(0, 60);
}

const kinds: KindConfig[] = [
  {
    label: "condition",
    segment: "conditions",
    listTitle: "Conditions",
    createLabel: "Record a condition",
    formLabelCreate: "Record a condition",
    formLabelEdit: "Edit condition",
    detailLandmark: "Condition",
    fields: (name) => [
      { label: "Diagnosis", kind: "text", value: name },
      { label: "State", kind: "select", value: "Active" },
    ],
    correctionLabel: "Diagnosis",
    correctionValue: (name) => `${name} corrected`,
    headingCheck: (article) => article.getByRole("heading", { level: 1 }),
  },
  {
    label: "encounter",
    segment: "encounters",
    listTitle: "Encounters",
    createLabel: "Record an encounter",
    formLabelCreate: "Record an encounter",
    formLabelEdit: "Edit encounter",
    detailLandmark: "Encounter",
    fields: (name) => [
      { label: "Reason for the visit", kind: "text", value: name },
      { label: "Date", kind: "date", value: "2025-06-01" },
    ],
    correctionLabel: "Reason for the visit",
    correctionValue: (name) => `${name} corrected`,
    headingCheck: (article) => article.getByRole("heading", { level: 1 }),
  },
  {
    label: "vitals",
    segment: "vitals",
    listTitle: "Measurements",
    createLabel: "Record measurements",
    formLabelCreate: "Record measurements",
    formLabelEdit: "Edit measurements",
    detailLandmark: "Measurement set",
    fields: () => [
      { label: "When", kind: "datetime", value: "2025-06-01T07:00" },
      { label: "Heart rate (bpm)", kind: "number", value: "72" },
    ],
    correctionLabel: "Device",
    correctionValue: (name) => name,
    headingCheck: (article, corrected) =>
      article
        .locator("dt", { hasText: "Device" })
        .locator("xpath=following-sibling::dd[1]")
        .filter({ hasText: corrected }),
  },
  {
    label: "vaccination",
    segment: "immunizations",
    listTitle: "Vaccinations",
    createLabel: "Record a vaccination",
    formLabelCreate: "Record a vaccination",
    formLabelEdit: "Edit vaccination",
    detailLandmark: "Vaccination",
    fields: (name) => [
      { label: "Vaccine", kind: "text", value: name },
      { label: "Given on", kind: "date", value: "2025-01-01" },
    ],
    correctionLabel: "Vaccine",
    correctionValue: (name) => `${name} corrected`,
    headingCheck: (article) => article.getByRole("heading", { level: 1 }),
  },
  {
    label: "insurance policy",
    segment: "insurance",
    listTitle: "Insurance",
    createLabel: "Record a policy",
    formLabelCreate: "Record an insurance policy",
    formLabelEdit: "Edit insurance policy",
    detailLandmark: "Insurance",
    fields: (name) => [
      { label: "Insurer", kind: "text", value: name },
      { label: "Kind", kind: "select", value: "Medical" },
      { label: "Member name", kind: "text", value: "Keyboard Rehearsal" },
      { label: "Member ID", kind: "text", value: `KB-${name.slice(-8)}` },
      { label: "Relationship to holder", kind: "select", value: "Self" },
      { label: "Cover starts", kind: "date", value: "2025-01-01" },
      { label: "State", kind: "select", value: "Active" },
    ],
    correctionLabel: "Insurer",
    correctionValue: (name) => `${name} corrected`,
    headingCheck: (article) => article.getByRole("heading", { level: 1 }),
  },
  {
    label: "relative",
    segment: "family-history",
    listTitle: "Family history",
    createLabel: "Record a relative",
    formLabelCreate: "Record a relative",
    formLabelEdit: "Edit relative",
    detailLandmark: "Relative",
    fields: (name) => [
      { label: "Name", kind: "text", value: name },
      { label: "Relationship", kind: "select", value: "Mother" },
    ],
    correctionLabel: "Name",
    correctionValue: (name) => `${name} corrected`,
    headingCheck: (article) => article.getByRole("heading", { level: 1 }),
  },
];

for (const kind of kinds) {
  test.describe(`keyboard-only ${kind.label}`, () => {
    test.use({ signedInAs: "anonymous" });

    test(`records, corrects and deletes a ${kind.label} with the keyboard alone`, async ({
      page,
    }) => {
      await newAccount(page);
      await newPatient(page);

      const name = scratch(test.info().project.name, kind.label);
      const tagName = scratch(
        test.info().project.name,
        `${kind.label} tag`,
      ).slice(0, 40);
      await newTag(page, tagName);

      await page.goto(`/${kind.segment}`);
      await expect(page).toHaveTitle(fixtures.title(kind.listTitle));

      const createLink = page
        .getByRole("link", { name: kind.createLabel })
        .first();
      await expect(createLink).toBeVisible();
      await activate(createLink, kind.createLabel);

      const createForm = page.getByRole("form", { name: kind.formLabelCreate });
      await expect(createForm).toBeVisible();

      for (const field of kind.fields(name)) {
        const control = createForm.getByLabel(field.label, { exact: true });
        switch (field.kind) {
          case "select":
            await selectByTyping(control, field.label, field.value);
            break;
          case "date":
            await typeDate(control, field.label, field.value);
            break;
          case "datetime":
            await typeDateTime(control, field.label, field.value);
            break;
          default:
            await typeText(control, field.label, field.value);
        }
      }

      const submit = createForm.getByRole("button", { name: "Record it" });
      await activate(submit, "Record it");

      const rows = page.locator("tbody tr");
      await expect(rows).toHaveCount(1);
      if (kind.correctionLabel !== "Device") {
        await expect(rows.first()).toContainText(name);
      }

      const rowLink = rows.first().getByRole("link").first();
      await activate(rowLink, `the new ${kind.label}'s row`);

      const article = page.getByRole("article", { name: kind.detailLandmark });
      await expect(article).toBeVisible();

      const editLink = article.getByRole("link", { name: "Edit" });
      await activate(editLink, "Edit");

      const editForm = page.getByRole("form", { name: kind.formLabelEdit });
      await expect(editForm).toBeVisible();

      const corrected = kind.correctionValue(name);
      const correctionControl = editForm.getByLabel(kind.correctionLabel, {
        exact: true,
      });
      await typeText(correctionControl, kind.correctionLabel, corrected);

      await applyTag(editForm, tagName);

      const save = editForm.getByRole("button", { name: "Save changes" });
      await activate(save, "Save changes");

      await expect(kind.headingCheck(article, corrected)).toBeVisible();
      // vitals titles its detail page after the recorded timestamp, not
      // after the Device field this test corrects, so there is no page
      // title assertion to make from the corrected value on that one kind.
      if (kind.correctionLabel !== "Device") {
        await expect(page).toHaveTitle(fixtures.title(corrected));
      }

      // The tag applied above round-tripped through the save: the
      // re-rendered form (patched in place, same as the correction) still
      // shows it as a chip rather than reverting to blank.
      await expect(
        editForm.getByRole("button", { name: `Remove ${tagName}` }),
      ).toBeVisible();

      // T215: the "relate" step is US6, not on this branch (see file header).

      const deleteButton = article.getByRole("button", { name: "Delete" });
      await activate(deleteButton, "Delete");

      const confirm = page.getByRole("region", { name: "Confirm delete" });
      await expect(confirm).toBeVisible();

      // Cancel first, proving focus is not lost into the now-closed drawer -
      // the failure mode SC-018 names by name.
      const cancel = confirm.getByRole("button", { name: "Keep it" });
      await activate(cancel, "Keep it");
      await expect(confirm).toBeHidden();

      const afterCancel = await page.evaluate(() => {
        const element = document.activeElement;
        return element
          ? {
              tag: element.tagName,
              visible: element.checkVisibility?.() ?? true,
            }
          : null;
      });
      expect(
        afterCancel,
        "focus was lost when the delete confirmation closed",
      ).not.toBeNull();
      expect(
        afterCancel?.tag,
        "focus fell back to <body>, not a visible control",
      ).not.toBe("BODY");
      expect(
        afterCancel?.visible,
        "focus landed on a control that is not visible",
      ).toBe(true);

      await activate(deleteButton, "Delete");
      await expect(confirm).toBeVisible();

      const confirmDelete = confirm.getByRole("button", {
        name: "Delete permanently",
      });
      await activate(confirmDelete, "Delete permanently");

      await page.goto(`/${kind.segment}`);
      await expect(page.locator("tbody tr")).toHaveCount(0);
    });
  });
}
