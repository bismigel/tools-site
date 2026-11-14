import { expect, test } from "@playwright/test";

test.describe("slugify with JavaScript", () => {
  test("htmx swaps the result without navigating", async ({ page }) => {
    await page.goto("/t/slugify");
    await page.getByLabel("Text").fill("Hello, World!");
    await page.getByRole("button", { name: "Run" }).click();

    // The fragment lands in #result and the document is not replaced: this
    // marker survives only if no navigation happened.
    await page.evaluate(() => ((window as any).__marker = "kept"));
    await page.getByLabel("Text").fill("Second run");
    await page.getByRole("button", { name: "Run" }).click();

    await expect(page.locator("#result")).toContainText("second-run");
    expect(await page.evaluate(() => (window as any).__marker)).toBe("kept");
    expect(new URL(page.url()).pathname).toBe("/t/slugify");
  });

  test("server-side validation re-renders the form and keeps the page", async ({ page }) => {
    await page.goto("/t/slugify");
    // Native `required` blocks an empty submit before the server sees it, so use
    // a value the browser accepts and the contract rejects (5000-rune cap).
    await page.getByLabel("Text").fill("a".repeat(5001));
    await page.getByRole("button", { name: "Run" }).click();

    await expect(page.locator("#tool-form form")).toHaveAttribute("data-invalid", "true");
    await expect(page.getByText(/at most 5000 characters/)).toBeVisible();
    await expect(page.locator("#result")).not.toContainText("Slug");
  });

  test("an empty submit is stopped by the browser before any request", async ({ page }) => {
    await page.goto("/t/slugify");
    const posts: string[] = [];
    page.on("request", (r) => {
      if (r.method() === "POST") posts.push(r.url());
    });
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.getByLabel("Text")).toBeFocused();
    expect(posts).toEqual([]);
  });

  test("dark mode survives a reload", async ({ page }) => {
    await page.goto("/");
    const html = page.locator("html");
    const before = await html.getAttribute("data-scheme");

    await page.getByRole("button", { name: /theme/i }).click();
    const after = await html.getAttribute("data-scheme");
    expect(after).not.toBe(before);

    await page.reload();
    await expect(html).toHaveAttribute("data-scheme", after!);
  });
});

test.describe("slugify without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("the form posts normally and returns a full page", async ({ page }) => {
    await page.goto("/t/slugify");
    await page.getByLabel("Text").fill("Crème Brûlée");
    await page.getByRole("button", { name: "Run" }).click();

    await expect(page.locator("#result")).toContainText("creme-brulee");
    await expect(page.locator("#result").getByRole("heading", { name: "Slug", exact: true })).toBeVisible();
  });
});
