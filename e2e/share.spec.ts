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

});

test.describe("htmx behaviour", () => {
  test("a submit swaps the fragment without navigating", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount", { exact: true }).fill("550");
    await page.getByLabel("Currency", { exact: true }).selectOption("GBP");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#result")).toContainText("per year");

    // Survives only if the document was not replaced by a navigation.
    await page.evaluate(() => ((window as any).__marker = "kept"));
    await page.getByLabel("Amount", { exact: true }).fill("300");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#result")).toContainText("£69,000.00");
    expect(await page.evaluate(() => (window as any).__marker)).toBe("kept");
  });

  test("an empty submit is stopped by the browser, not the server", async ({ page }) => {
    await page.goto("/t/rate_converter");
    const calls: string[] = [];
    page.on("request", (r) => {
      if (new URL(r.url()).pathname === "/t/rate_converter") calls.push(r.url());
    });
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.getByLabel("Amount", { exact: true })).toBeFocused();
    expect(calls).toEqual([]);
  });

  test("the Run button cannot double-submit", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount", { exact: true }).fill("550");
    await page.getByLabel("Currency", { exact: true }).selectOption("GBP");

    const calls: string[] = [];
    page.on("request", (r) => {
      // GET tools carry their input in the query string.
      if (new URL(r.url()).pathname === "/t/rate_converter") calls.push(r.url());
    });

    await page.getByRole("button", { name: "Run" }).dblclick();
    await expect(page.locator("#result")).toContainText("per year");
    expect(calls.length).toBe(1);
  });
});
