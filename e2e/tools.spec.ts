import { expect, test } from "@playwright/test";

test.describe("rate_converter", () => {
  test("normalises £550/day to five periods and to EUR", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount", { exact: true }).fill("550");
    await page.getByLabel("Currency", { exact: true }).selectOption("GBP");
    await page.getByLabel("Quoted per", { exact: true }).selectOption("day");
    await page.getByLabel("Convert to", { exact: true }).selectOption("EUR");
    await page.getByRole("button", { name: "Run" }).click();

    const table = page.locator("#result table");
    await expect(table).toBeVisible();
    await expect(table).toContainText("Annual");
    await expect(table).toContainText("£126,500.00");

    // The assumptions and the rate date are part of the answer.
    await expect(page.locator("#result")).toContainText("1,840 hours/year");
    await expect(page.locator("#result")).toContainText("ECB reference rate");
  });

  test("compares two offers side by side", async ({ page }) => {
    await page.goto("/t/rate_converter?amount=550&currency=GBP&period=day&target=GBP&amount2=85&currency2=EUR&period2=hour");

    const table = page.locator("#result table");
    await expect(table).toContainText("Offer A");
    await expect(table).toContainText("Offer B");
    await expect(table).toContainText("£130,686.28");
    await expect(page.locator("#result")).toContainText("3.3% higher per year");
  });

  test("rejects a zero amount with a field message", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount", { exact: true }).fill("0");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#tool-form form")).toHaveAttribute("data-invalid", "true");
    await expect(page.getByText(/greater than zero/i)).toBeVisible();
  });
});

test.describe("meeting_planner", () => {
  test("finds the London/New York overlap", async ({ page }) => {
    await page.goto("/t/meeting_planner?city1=London&city2=New%20York&date=2026-11-16&length=60");

    await expect(page.locator("#result")).toContainText("3 hours overlap");
    await expect(page.locator("#result")).toContainText("2:00 PM–5:00 PM London");
    await expect(page.locator("#result")).toContainText("9:00 AM–12:00 PM New York");
    await expect(page.locator("#result table tbody tr")).toHaveCount(24);
  });

  test("shows the current time and marks the hour", async ({ page }) => {
    await page.goto("/t/meeting_planner?city1=Dubai&city2=Manila"); // default date = today
    await expect(page.locator("#result")).toContainText("Now");
    await expect(page.locator("#result .cell-note").filter({ hasText: /^now$/ })).toHaveCount(1);
  });

  test("renders 12-hour labels on request", async ({ page }) => {
    await page.goto("/t/meeting_planner?city1=London&city2=New%20York&date=2026-11-16&format=12");
    await expect(page.locator("#result")).toContainText("2:00 PM–5:00 PM London");
  });

  test("says plainly when there is no overlap", async ({ page }) => {
    await page.goto("/t/meeting_planner?city1=London&city2=Sydney&date=2026-11-16");
    await expect(page.locator("#result")).toContainText("No 60-minute overlap");
  });

  test("rejects an unknown city", async ({ page }) => {
    await page.goto("/t/meeting_planner?city1=London&city2=Atlantis&date=2026-11-16");
    await expect(page.getByText(/Atlantis/)).toBeVisible();
  });
});

test.describe("time zone detection", () => {
  test("defaults the anchor to the visitor's zone", async ({ browser }) => {
    const context = await browser.newContext({ timezoneId: "Europe/Lisbon" });
    const page = await context.newPage();
    await page.goto("/t/meeting_planner");

    await expect(page.locator("#result")).toContainText("Lisbon");
    await expect(page).toHaveURL(/city1=Europe%2FLisbon/);
    await context.close();
  });

  test("an explicit anchor in the URL always wins", async ({ browser }) => {
    const context = await browser.newContext({ timezoneId: "Europe/Lisbon" });
    const page = await context.newPage();
    await page.goto("/t/meeting_planner?city1=Dubai&city2=Manila");

    await expect(page.locator("#result")).toContainText("Dubai");
    await expect(page).not.toHaveURL(/Lisbon/);
    await context.close();
  });

  test("without JavaScript the default anchor stands", async ({ browser }) => {
    const context = await browser.newContext({ timezoneId: "Europe/Lisbon", javaScriptEnabled: false });
    const page = await context.newPage();
    await page.goto("/t/meeting_planner");

    await expect(page.getByLabel("City 1 (anchor)", { exact: true })).toHaveValue("Dubai");
    await context.close();
  });
});
