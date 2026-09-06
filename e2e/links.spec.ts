// T142, T150. FR-059's linked-records rendering and FR-060/FR-061's course-
// medication rendering, both demonstrated on the one seeded treatment that
// carries a real Condition relation and one attached course medication
// (internal/testsupport/seed/care.go, internal/testsupport/seed/links.go).
//
// views.LinkedRecords and views.CourseMedications render as their own
// sections, siblings of the detail article rather than nested inside it
// (internal/web/page/treatments.go's sequence), so they are found off the
// page and not off the article landmark open() returns.
import { open } from "./gate";
import { expect, test } from "./auth";
import { fixtures } from "./fixtures";

const treatmentName = "Cardiac rehabilitation";

test.describe("the treatment detail page", () => {
  test("renders its condition as an openable link", async ({ page }) => {
    await open(page, {
      path: fixtures.treatment.detailPath(fixtures.treatment.linkedID),
      title: fixtures.title(treatmentName),
      landmark: { role: "article", name: "Treatment" },
    });

    const linked = page.getByRole("region", { name: "Linked records" });
    await expect(linked).toBeVisible();

    const conditionHref = fixtures.condition.detailPath(
      fixtures.condition.seededID,
    );
    const anchor = linked.locator(`a[href="${conditionHref}"]`);
    await expect(anchor).toBeVisible();
  });

  test("renders its attached course medication with an effective value", async ({
    page,
  }) => {
    await open(page, {
      path: fixtures.treatment.detailPath(fixtures.treatment.linkedID),
      title: fixtures.title(treatmentName),
      landmark: { role: "article", name: "Treatment" },
    });

    const courseMedications = page.getByRole("region", {
      name: "Course medications",
    });
    await expect(courseMedications).toBeVisible();

    const medicationHref = fixtures.detailPath(
      fixtures.treatment.linkedMedicationID,
    );
    const anchor = courseMedications.locator(`a[href="${medicationHref}"]`);
    await expect(anchor).toBeVisible();

    // The join's own dosage, seeded by internal/testsupport/seed/links.go, so
    // the section is proven to render a real effective value and not just an
    // empty state that happens to contain the medication's link.
    await expect(courseMedications).toContainText("10 mg");
  });

  test("a treatment with neither relation renders both empty states", async ({
    page,
  }) => {
    await open(page, {
      path: fixtures.treatment.detailPath(fixtures.treatment.seededID),
      title: fixtures.title("Physical therapy"),
      landmark: { role: "article", name: "Treatment" },
    });

    const linked = page.getByRole("region", { name: "Linked records" });
    await expect(linked).toBeVisible();
    await expect(linked.getByRole("link")).toHaveCount(0);

    const courseMedications = page.getByRole("region", {
      name: "Course medications",
    });
    await expect(courseMedications).toBeVisible();
    await expect(courseMedications.getByRole("link")).toHaveCount(0);
  });
});
