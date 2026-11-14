import { expect, Page, test } from "@playwright/test";

// A stub modelContext, installed before any page script runs. This exercises the
// real bridge, the real fetch and the real API in a real browser — the closest CI
// can get to a WebMCP-capable client.
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
    await page.goto("/t/rate_converter");

    await expect.poll(() => page.evaluate(() => (window as any).__mcp.calls)).toBe(1);

    const names = await page.evaluate(() =>
      ((window as any).__mcp.tools as Array<{ name: string }>).map((t) => t.name)
    );
    expect(names).toEqual(["rate_converter", "meeting_planner"]);
  });

  test("execute returns the same text the page shows", async ({ page }) => {
    await stubWebMCP(page);
    await page.goto("/t/rate_converter");
    await expect.poll(() => page.evaluate(() => (window as any).__mcp.calls)).toBe(1);

    const text = await page.evaluate(async () => {
      const tool = ((window as any).__mcp.tools as any[]).find((t) => t.name === "rate_converter");
      const out = await tool.execute({ amount: "550", currency: "GBP", period: "day", target: "EUR" });
      return out.content[0].text as string;
    });
    expect(text).toContain("€151,390.04");

    // The page shows the same sentence for the same input.
    await page.goto("/t/rate_converter?amount=550&currency=GBP&period=day&target=EUR");
    await expect(page.locator("#result")).toContainText("€151,390.04");
  });

  test("a failing call is isError, never an exception", async ({ page }) => {
    await stubWebMCP(page);
    await page.goto("/t/rate_converter");
    await expect.poll(() => page.evaluate(() => (window as any).__mcp.calls)).toBe(1);

    const out = await page.evaluate(async () => {
      const tool = ((window as any).__mcp.tools as any[]).find((t) => t.name === "rate_converter");
      const res = await tool.execute({ amount: "0", currency: "GBP", period: "day", target: "GBP" });
      return { isError: res.isError === true, text: res.content[0].text as string };
    });
    expect(out.isError).toBe(true);
    expect(out.text.toLowerCase()).toContain("zero");
  });

  test("without the API the page works and nothing extra is fetched", async ({ page }) => {
    const paths: string[] = [];
    page.on("request", (r) => paths.push(new URL(r.url()).pathname));

    await page.goto("/t/rate_converter");
    await page.getByLabel("Amount", { exact: true }).fill("550");
    await page.getByLabel("Currency", { exact: true }).selectOption("GBP");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.locator("#result")).toContainText("per year");

    expect(paths).not.toContain("/api/tools");
    expect(paths).toContain("/t/rate_converter");
  });
});
