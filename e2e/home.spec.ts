import { expect, test } from "@playwright/test";

test.describe("home", () => {
  test("lists the tools and links to each one", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveTitle(/Tools/);

    const cards = page.locator(".article-list article");
    await expect(cards).toHaveCount(3);

    for (const name of ["slugify", "rate_converter", "meeting_planner"]) {
      await expect(page.locator(`a[href="/t/${name}"]`)).toBeVisible();
    }
  });

  test("serves a themed 404", async ({ page }) => {
    const res = await page.goto("/nope");
    expect(res?.status()).toBe(404);
    await expect(page.getByRole("heading", { name: /not found/i })).toBeVisible();
  });

  test("sends the security headers on every response", async ({ request }) => {
    const res = await request.get("/");
    expect(res.headers()["content-security-policy"]).toContain("default-src 'none'");
    expect(res.headers()["x-content-type-options"]).toBe("nosniff");
    expect(res.headers()["referrer-policy"]).toBe("strict-origin-when-cross-origin");
  });

  test("has no console errors or page errors on load", async ({ page }) => {
    const problems: string[] = [];
    page.on("console", (m) => {
      if (m.type() === "error") problems.push(`console: ${m.text()}`);
    });
    page.on("pageerror", (e) => problems.push(`pageerror: ${e.message}`));

    await page.goto("/");
    await page.goto("/t/slugify");
    expect(problems).toEqual([]);
  });
});
