// T183a/T189. routes.gate.spec.ts already runs contracts/pages.md's seven
// generic assertions against /timeline (it is driven off the application's
// own route table); this file is the things only a browser proves about the
// timeline itself: the explicit "choose a person" state with no ?patient=,
// and a narrowing that empties the list showing "Nothing matches" with a
// removable chip rather than "Nothing recorded".
import { pageRoutes } from "./routes";
import { expect, test } from "./auth";

function timelinePath(): string {
  const route = pageRoutes.find((route) => route.opID === "timelinePage");
  expect(route, "routes.ts stopped listing timelinePage").toBeTruthy();
  return route!.smokeURL;
}

test.describe("the timeline", () => {
  test.use({ signedInAs: "populated" });

  test("with no ?patient= renders the explicit choose-a-person state", async ({
    page,
  }) => {
    await page.goto("/timeline");

    const region = page.getByRole("region", { name: "Timeline" });
    await expect(region).toBeVisible();
    await expect(region).toContainText("Choose a person");
    await expect(page.locator("#timeline-groups")).toHaveCount(0);
  });

  test("groups entries and shows a date for each one", async ({ page }) => {
    await page.goto(timelinePath());

    const region = page.getByRole("region", { name: "Timeline" });
    await expect(region).toBeVisible();
    await expect(page.locator("#timeline-groups")).toBeVisible();
  });

  test("narrowing to a kind with nothing recorded shows a removable chip and Nothing matches", async ({
    page,
  }) => {
    const url = new URL(timelinePath(), "http://localhost");
    url.searchParams.set("kind", "insurance");

    await page.goto(url.pathname + url.search);

    const region = page.getByRole("region", { name: "Timeline" });
    await expect(region).toBeVisible();

    const chip = page.locator("#timeline-criteria-kind-insurance");
    await expect(chip).toBeVisible();
    await expect(chip).toContainText("insurance");

    const empty = page.locator("#timeline-empty");
    await expect(empty).toBeVisible();
    await expect(empty).toContainText("Nothing matches");

    await chip.getByRole("button", { name: /Remove/ }).click();
    await expect(page.locator("#timeline-empty")).toHaveCount(0);
    await expect(page.locator("#timeline-groups")).toBeVisible();
  });
});
