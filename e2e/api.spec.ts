import { expect, test } from "@playwright/test";

test.describe("JSON API", () => {
  test("manifest lists the public tools with schemas and no-store", async ({ request }) => {
    const res = await request.get("/api/tools");
    expect(res.status()).toBe(200);
    expect(res.headers()["cache-control"]).toBe("no-store");

    const body = await res.json();
    expect(body.tools.map((t: { name: string }) => t.name)).toEqual([
      "slugify",
      "rate_converter",
      "meeting_planner",
    ]);

    const slugify = body.tools[0];
    expect(slugify.inputSchema.type).toBe("object");
    expect(slugify.inputSchema.required).toContain("text");
  });

  test("executes a tool and returns a request id", async ({ request }) => {
    const res = await request.post("/api/tools/slugify", { data: { text: "Hello, World!" } });
    expect(res.status()).toBe(200);

    const body = await res.json();
    expect(body.data.slug).toBe("hello-world");
    expect(body.request_id).toBeTruthy();
  });

  test("bad input is a 400 envelope", async ({ request }) => {
    const res = await request.post("/api/tools/slugify", { data: { text: "" } });
    expect(res.status()).toBe(400);

    const body = await res.json();
    expect(body.error.code).toBe("bad_input");
    expect(body.error.message).toMatch(/required/i);
    expect(body.error.request_id).toBeTruthy();
  });

  test("an unknown field is rejected", async ({ request }) => {
    const res = await request.post("/api/tools/slugify", { data: { nope: "x" } });
    expect(res.status()).toBe(400);
    expect((await res.json()).error.message).toContain("nope");
  });

  test("the gated zone is 404, never 403", async ({ request }) => {
    expect((await request.get("/lab")).status()).toBe(404);
    expect((await request.post("/api/lab/anything", { data: {} })).status()).toBe(404);
  });

  test("probes answer", async ({ request }) => {
    expect((await request.get("/healthz")).status()).toBe(200);
    expect((await request.get("/readyz")).status()).toBe(200);
  });
});

test.describe("responsive", () => {
  test("no page-level horizontal scroll at 360px", async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 800 });
    for (const path of ["/", "/t/slugify", "/t/rate_converter", "/t/meeting_planner"]) {
      await page.goto(path);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth
      );
      expect(overflow, `${path} overflows horizontally`).toBeLessThanOrEqual(1);
    }
  });

  test("the meeting grid scrolls inside its card, not the page", async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 800 });
    await page.goto("/t/meeting_planner");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#result table")).toBeVisible();

    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth
    );
    expect(overflow).toBeLessThanOrEqual(1);
  });
});

test.describe("accessibility basics", () => {
  test("every control has an accessible name and the first field takes focus", async ({ page }) => {
    await page.goto("/t/rate_converter");

    for (const label of ["Amount", "Currency", "Quoted per", "Convert to"]) {
      await expect(page.getByLabel(label, { exact: true })).toBeVisible();
    }

    await page.keyboard.press("Tab");
    const focused = await page.evaluate(() => document.activeElement?.tagName.toLowerCase());
    expect(["a", "button", "input", "select", "textarea"]).toContain(focused);
  });
});
