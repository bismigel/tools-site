import { expect, Page, test } from "@playwright/test";

// A stub modelContext, installed before any page script runs. This exercises
// the real bridge, the real fetch and the real API in a real browser — the
// closest CI can get to a WebMCP-capable client.
async function stubWebMCP(page: Page) {
  await page.addInitScript(() => {
    (window as any).__mcp = { tools: null, calls: 0 };
    Object.defineProperty(navigator, "modelContext", {
      configurable: true,
      value: {
        provideContext: (ctx: { tools: unknown }) => {
          (window as any).__mcp.tools = ctx.tools;
          (window as any).__mcp.calls += 1;
        },
      },
    });
  });
}

test.describe("WebMCP bridge", () => {
  test("registers the public tools exactly once", async ({ page }) => {
    await stubWebMCP(page);
    await page.goto("/t/slugify");

    await expect.poll(() => page.evaluate(() => (window as any).__mcp.calls)).toBe(1);

    const names = await page.evaluate(() =>
      ((window as any).__mcp.tools as Array<{ name: string }>).map((t) => t.name)
    );
    expect(names).toEqual(["slugify", "rate_converter", "meeting_planner"]);
  });

  test("execute returns the same text as the page shows", async ({ page }) => {
    await stubWebMCP(page);
    await page.goto("/t/slugify");
    await expect.poll(() => page.evaluate(() => (window as any).__mcp.calls)).toBe(1);

    const text = await page.evaluate(async () => {
      const tool = ((window as any).__mcp.tools as any[]).find((t) => t.name === "slugify");
      const out = await tool.execute({ text: "Hello, World!" });
      return out.content[0].text as string;
    });
    expect(text).toBe("hello-world");

    // And the page itself shows the same thing for the same input.
    await page.getByLabel("Text").fill("Hello, World!");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#result")).toContainText("hello-world");
  });

  test("a failing call is isError, never an exception", async ({ page }) => {
    await stubWebMCP(page);
    await page.goto("/t/slugify");
    await expect.poll(() => page.evaluate(() => (window as any).__mcp.calls)).toBe(1);

    const out = await page.evaluate(async () => {
      const tool = ((window as any).__mcp.tools as any[]).find((t) => t.name === "slugify");
      const res = await tool.execute({ text: "" });
      return { isError: res.isError === true, text: res.content[0].text as string };
    });
    expect(out.isError).toBe(true);
    expect(out.text.toLowerCase()).toContain("required");
  });

  test("without the API the page works and nothing extra is fetched", async ({ page }) => {
    const paths: string[] = [];
    page.on("request", (r) => paths.push(new URL(r.url()).pathname));

    await page.goto("/t/slugify");
    await page.getByLabel("Text").fill("No WebMCP browser");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#result")).toContainText("no-webmcp-browser");

    expect(paths).not.toContain("/api/tools");
    expect(paths).toContain("/t/slugify");
  });
});
