import { expect, test } from "@playwright/test";

test.describe("home", () => {
  test("lists the tools and links to each one", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveTitle(/Tools/);

    const cards = page.locator(".article-list article");
    await expect(cards).toHaveCount(2);

    for (const name of ["rate_converter", "meeting_planner"]) {
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
    await page.goto("/t/rate_converter");
    expect(problems).toEqual([]);
  });
});

test.describe("page furniture", () => {
  test("tool titles are written for people, not identifiers", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Rate converter", exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "rate_converter", exact: true })).toHaveCount(0);
  });

  test("the whole card is the link, and the tool page has one h1", async ({ page }) => {
    await page.goto("/");
    await page.locator('a.tool-card[href="/t/meeting_planner"]').click();
    // The planner now fills the anchor from the browser zone, so a query is expected.
    await expect(page).toHaveURL(/\/t\/meeting_planner(\?|$)/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Meeting planner");
  });

  test("the 404 page offers the tools instead of a dead end", async ({ page }) => {
    await page.goto("/nope");
    for (const name of ["Rate converter", "Meeting planner"]) {
      await expect(page.getByRole("link", { name, exact: true })).toBeVisible();
    }
  });

  test("the first tab stop is the skip link", async ({ page }) => {
    await page.goto("/");
    await page.keyboard.press("Tab");
    await expect(page.locator(".skip-link")).toBeFocused();
  });

  test("the empty result card explains what will happen", async ({ page }) => {
    await page.goto("/t/rate_converter");
    await expect(page.locator("#result")).toContainText("Fill this in");
  });
});

test.describe("dark mode", () => {
  // WCAG relative luminance + contrast ratio, so this is a real readability check
  // and not a colour comparison that passes by accident.
  function luminance(rgb: string): number {
    const [r, g, b] = (rgb.match(/\d+/g) || ["0", "0", "0"]).slice(0, 3).map(Number);
    const chan = [r, g, b].map((v) => {
      const s = v / 255;
      return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
    });
    return 0.2126 * chan[0] + 0.7152 * chan[1] + 0.0722 * chan[2];
  }
  function contrast(a: string, b: string): number {
    const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
    return (hi + 0.05) / (lo + 0.05);
  }

  for (const scheme of ["dark", "light"] as const) {
    test(`${scheme} mode body text meets AA against the page`, async ({ browser }) => {
      const ctx = await browser.newContext({ colorScheme: scheme });
      const page = await ctx.newPage();
      await page.goto("/");

      const colours = await page.evaluate(() => {
        const body = getComputedStyle(document.body);
        const title = getComputedStyle(document.querySelector(".page-title")!);
        return { text: body.color, bg: body.backgroundColor, title: title.color };
      });

      expect(colours.bg).not.toBe("rgba(0, 0, 0, 0)");
      expect(contrast(colours.text, colours.bg)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(colours.title, colours.bg)).toBeGreaterThanOrEqual(4.5);
      await ctx.close();
    });
  }
});
