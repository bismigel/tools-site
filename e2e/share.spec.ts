import { expect, test } from "@playwright/test";

// NFR-7.1: the URL is the answer. A result must survive a copy-paste, a reload
// and a colleague opening it.
test.describe("shareable results", () => {
  test("a GET tool runs from its URL alone", async ({ page }) => {
    await page.goto("/t/rate_converter?amount=550&currency=GBP&period=day&target=EUR");

    await expect(page.locator("#result table")).toBeVisible();
    await expect(page.locator("#result")).toContainText("€151,390.04");
    await expect(page.locator("#result")).toContainText("per year");
  });

  test("the result survives a reload and a fresh tab", async ({ page, context }) => {
    const url = "/t/meeting_planner?city1=London&city2=New%20York&date=2026-11-16&length=60";
    await page.goto(url);
    await expect(page.locator("#result")).toContainText("3 hours overlap");

    await page.reload();
    await expect(page.locator("#result")).toContainText("3 hours overlap");

    const other = await context.newPage();
    await other.goto(url);
    await expect(other.locator("#result")).toContainText("3 hours overlap");
    await other.close();
  });

  test("submitting keeps the inputs in the URL", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await expect(page.locator("form")).toHaveAttribute("method", "get");

    await page.getByLabel("Amount", { exact: true }).fill("550");
    await page.getByLabel("Currency", { exact: true }).selectOption("GBP");
    await page.getByRole("button", { name: "Run" }).click();

    await expect(page.locator("#result")).toContainText("£126,500.00");
    await expect.poll(() => new URL(page.url()).searchParams.get("amount")).toBe("550");
  });

  test("the long-text tool stays a POST, because a slug request does not fit in a URL", async ({ page }) => {
    await page.goto("/t/slugify");
    await expect(page.locator("form")).toHaveAttribute("method", "post");
  });

  test("every tool page still works with JavaScript disabled", async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled: false });
    const page = await context.newPage();

    await page.goto("/t/rate_converter?amount=550&target=EUR&period=day");
    await expect(page.locator("#result table")).toBeVisible();

    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount", { exact: true }).fill("300");
    await page.getByLabel("Currency", { exact: true }).selectOption("GBP");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#result")).toContainText("£69,000.00");

    await context.close();
  });
});

test.describe("form layout", () => {
  test("short fields share a row on wide screens and stack on narrow ones", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/t/rate_converter");

    const amount = await page.getByLabel("Amount", { exact: true }).boundingBox();
    const currency = await page.getByLabel("Currency", { exact: true }).boundingBox();
    expect(amount && currency).toBeTruthy();
    // Same row, different column: that is the grid doing its job.
    expect(Math.abs(amount!.y - currency!.y)).toBeLessThan(2);
    expect(currency!.x).toBeGreaterThan(amount!.x + 20);

    await page.setViewportSize({ width: 390, height: 844 });
    await page.reload();
    const narrowAmount = await page.getByLabel("Amount", { exact: true }).boundingBox();
    const narrowCurrency = await page.getByLabel("Currency", { exact: true }).boundingBox();
    expect(narrowCurrency!.y).toBeGreaterThan(narrowAmount!.y + 20);
  });

  test("a textarea always takes the full row", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/t/slugify");
    const box = await page.getByLabel("Text", { exact: true }).boundingBox();
    expect(box!.width).toBeGreaterThan(600);
  });
});
