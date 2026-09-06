// T163. contracts/tags.md §6's browser-only assertion: the delete
// confirmation states how many records carry the tag before it may be
// confirmed, and a rename shows up with no navigation. routes.gate.spec.ts
// already runs contracts/pages.md's seven generic assertions against /tags
// (it is driven off the application's own route table); this file is the
// two things only a browser proves about the tag manager itself.
import { fixtures } from "./fixtures";
import { open } from "./gate";
import { expect, test } from "./auth";

// Both viewport projects run against one instance, so the scratch names carry
// the project they belong to; a tag name is unique per account (FR-063).
const scratch = (suffix: string) =>
  `e2e-${test.info().project.name.replace(/[^a-z0-9]/gi, "")}-${suffix}`.slice(
    0,
    40,
  );

test.describe("the tag manager", () => {
  test("creating, renaming and deleting a tag needs no navigation", async ({
    page,
  }) => {
    const createdName = scratch("tag");
    const renamedName = scratch("renamed");
    const manager = await open(page, {
      path: "/tags",
      title: fixtures.title("Tags"),
      landmark: { role: "region", name: "Tags" },
    });

    const createForm = manager.getByRole("form", { name: "Add a tag" });
    await createForm.getByLabel("Name", { exact: true }).fill(createdName);
    await createForm.getByRole("button", { name: "Add tag" }).click();

    const row = manager.getByRole("listitem").filter({ hasText: createdName });
    await expect(row).toBeVisible();
    await expect(row).toContainText("0 records");

    await row.getByRole("button", { name: "Rename" }).click();
    const renameForm = row.getByRole("form", { name: `Rename ${createdName}` });
    await renameForm.getByLabel("Name", { exact: true }).fill(renamedName);
    await renameForm.getByRole("button", { name: "Save changes" }).click();

    const renamedRow = manager
      .getByRole("listitem")
      .filter({ hasText: renamedName });
    await expect(renamedRow).toBeVisible();
    await expect(
      manager.getByRole("listitem").filter({ hasText: createdName }),
    ).toHaveCount(0);

    // The delete confirmation states the consequence before it may be
    // confirmed (FR-066): zero records carry a tag nothing was ever tagged
    // with, and the confirmation says so rather than a generic warning.
    await renamedRow.getByRole("button", { name: "Delete" }).click();
    const confirm = renamedRow.locator('[aria-label="Confirm delete"]');
    await expect(confirm).toBeVisible();
    await expect(confirm).toContainText("No record carries this tag");

    await confirm.getByRole("button", { name: "Delete permanently" }).click();
    await expect(
      manager.getByRole("listitem").filter({ hasText: renamedName }),
    ).toHaveCount(0);
  });

  test("a tag already carried by a record states the count before delete", async ({
    page,
  }) => {
    const manager = await open(page, {
      path: "/tags",
      title: fixtures.title("Tags"),
      landmark: { role: "region", name: "Tags" },
    });

    const chronic = manager
      .getByRole("listitem")
      .filter({ hasText: "chronic" });
    await expect(chronic).toBeVisible();

    await chronic.getByRole("button", { name: "Delete" }).click();
    const confirm = chronic.locator('[aria-label="Confirm delete"]');
    await expect(confirm).toBeVisible();
    await expect(confirm).toContainText("records carry this tag");
    await expect(confirm).not.toContainText("No record carries this tag");

    // Cancelled rather than confirmed: this tag is the seeded fixture's own,
    // and a later spec run against the same worker needs it intact.
    await confirm.getByRole("button", { name: "Keep it" }).click();
    await expect(confirm).toBeHidden();
  });
});
