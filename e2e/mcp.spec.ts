import { expect, test } from "@playwright/test";

// MCP over streamable HTTP. Same registry, same schemas as /api/tools.
test.describe("MCP", () => {
  test("initialize advertises the tools capability", async ({ request }) => {
    const res = await request.post("/mcp", {
      data: { jsonrpc: "2.0", id: 1, method: "initialize" },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.result.protocolVersion).toBeTruthy();
    expect(body.result.capabilities.tools).toBeDefined();
    expect(body.result.serverInfo.name).toBe("tools-site");
  });

  test("tools/list matches the manifest", async ({ request }) => {
    const mcp = await (
      await request.post("/mcp", { data: { jsonrpc: "2.0", id: 2, method: "tools/list" } })
    ).json();
    const api = await (await request.get("/api/tools")).json();

    expect(mcp.result.tools.map((t: { name: string }) => t.name)).toEqual(
      api.tools.map((t: { name: string }) => t.name)
    );
    // Schemas are the same object, not a second definition.
    expect(mcp.result.tools[0].inputSchema).toEqual(api.tools[0].inputSchema);
  });

  test("tools/call returns text and structured content", async ({ request }) => {
    const res = await request.post("/mcp", {
      data: {
        jsonrpc: "2.0",
        id: 3,
        method: "tools/call",
        params: { name: "rate_converter", arguments: { amount: "550", currency: "GBP", period: "day", target: "EUR" } },
      },
    });
    expect(res.status()).toBe(200);

    const body = await res.json();
    expect(body.result.content[0].text).toContain("€151,390.04");
    expect(body.result.structuredContent.data.offers.a.values.year.target).toBe(151390.04);
    expect(body.result.isError).toBe(false);
  });

  test("a bad argument is a tool error, not a transport error", async ({ request }) => {
    const res = await request.post("/mcp", {
      data: {
        jsonrpc: "2.0",
        id: 4,
        method: "tools/call",
        params: { name: "rate_converter", arguments: { amount: "0", currency: "GBP", period: "day", target: "GBP" } },
      },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.result.isError).toBe(true);
    expect(body.result.content[0].text.toLowerCase()).toContain("zero");
  });

  test("an unknown tool is a JSON-RPC error", async ({ request }) => {
    const res = await request.post("/mcp", {
      data: { jsonrpc: "2.0", id: 5, method: "tools/call", params: { name: "nope", arguments: {} } },
    });
    expect((await res.json()).error.code).toBe(-32602);
  });

  test("GET /mcp is not a transport we speak", async ({ request }) => {
    expect((await request.get("/mcp")).status()).toBe(405);
  });
});

test.describe("metrics", () => {
  test("exposes request and tool counters in Prometheus text", async ({ request }) => {
    await request.get("/");
    await request.post("/api/tools/rate_converter", { data: { amount: "550", currency: "GBP", period: "day", target: "GBP" } });

    const res = await request.get("/metrics");
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toContain("text/plain");

    const body = await res.text();
    expect(body).toContain("# TYPE tools_http_requests_total counter");
    expect(body).toContain(`tools_tool_runs_total{tool="rate_converter",outcome="ok"}`);
    expect(body).toContain("tools_request_duration_seconds_count");
  });
});
