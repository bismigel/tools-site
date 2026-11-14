import { expect, test } from "@playwright/test";

test.describe("rate_converter", () => {
  test("normalises £550/day to five periods and to EUR", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount").fill("550");
    await page.getByLabel("Quoted per").selectOption("day");
    await page.getByLabel("Show in").selectOption("EUR");
    await page.getByRole("button", { name: "Run" }).click();

    const table = page.locator("#result table");
    await expect(table).toBeVisible();
    await expect(table).toContainText("Annual");
    await expect(table).toContainText("£126,500.00");

    // The assumptions and the rate date are part of the answer.
    await expect(page.locator("#result")).toContainText("1,840 hours/year");
    await expect(page.locator("#result")).toContainText("ECB reference rate");
  });

  test("rejects a zero amount with a field message", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount").fill("0");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#tool-form form")).toHaveAttribute("data-invalid", "true");
    await expect(page.getByText(/greater than zero/i)).toBeVisible();
  });
});

test.describe("meeting_planner", () => {
  test("finds the London/New York overlap", async ({ page }) => {
    await page.goto("/t/meeting_planner");
    await page.getByLabel("Date").fill("2026-11-16");
    await page.getByRole("button", { name: "Run" }).click();

    await expect(page.locator("#result")).toContainText("3 h overlap");
    await expect(page.locator("#result")).toContainText("14:00–17:00 London");
    await expect(page.locator("#result")).toContainText("New York 09:00–12:00");
    await expect(page.locator("#result table tbody tr")).toHaveCount(24);
  });

  test("says plainly when there is no overlap", async ({ page }) => {
    await page.goto("/t/meeting_planner");
    await page.getByLabel("Cities").fill("London, Sydney");
    await page.getByLabel("Date").fill("2026-11-16");
    await page.getByRole("button", { name: "Run" }).click();

    await expect(page.locator("#result")).toContainText("No 60-minute overlap");
  });

  test("rejects an unknown city", async ({ page }) => {
    await page.goto("/t/meeting_planner");
    await page.getByLabel("Cities").fill("London, Atlantis");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.getByText(/Atlantis/)).toBeVisible();
  });
});
