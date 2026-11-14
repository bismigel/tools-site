// Tests for web/static/js/webmcp.js, run with `node --test` or directly.
// No browser, no WebMCP implementation: we stub navigator.modelContext and
// fetch, then assert the contract from spec FR-5.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const here = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(join(here, "..", "web/static/js/webmcp.js"), "utf8");

const manifest = {
  tools: [
    {
      name: "slugify",
      description: "Turn text into a URL slug",
      inputSchema: {
        type: "object",
        properties: { text: { type: "string" } },
        required: ["text"]
      }
    }
  ]
};

function okBody(title, lines, warnings) {
  return { title, lines: lines || [], warnings: warnings || [] };
}

function load({ modelContext, fetchImpl, pathname = "/" }) {
  const calls = { fetched: [], provided: [], debug: [] };
  const sandbox = {
    navigator: modelContext === undefined ? {} : { modelContext },
    location: { pathname },
    console: {
      debug: (...args) => calls.debug.push(args.join(" ")),
      error: (...args) => calls.debug.push(args.join(" "))
    },
    fetch: (url, opts) => {
      calls.fetched.push({ url, opts });
      return fetchImpl(url, opts);
    },
    JSON,
    String,
    encodeURIComponent,
    Promise
  };
  vm.runInNewContext(source, sandbox, { filename: "webmcp.js" });
  return { calls, sandbox };
}

async function until(predicate, ms = 500) {
  const start = Date.now();
  while (Date.now() - start < ms) {
    if (predicate()) return;
    await new Promise((r) => setTimeout(r, 5));
  }
  throw new Error("condition not met in time");
}

async function test(name, fn) {
  try {
    await fn();
    console.log(`ok   ${name}`);
  } catch (err) {
    console.error(`FAIL ${name}`);
    console.error(err);
    process.exitCode = 1;
  }
}

await test("no modelContext: one debug line, no fetch, no throw", async () => {
  const { calls } = load({
    fetchImpl: () => {
      throw new Error("fetch must not be called");
    }
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(calls.fetched.length, 0);
  assert.equal(calls.debug.length, 1);
  assert.match(calls.debug[0], /WebMCP unavailable/);
});

await test("registers the manifest once, passing schemas through", async () => {
  let provided = null;
  const modelContext = { provideContext: (ctx) => { provided = ctx; } };
  const { calls } = load({
    modelContext,
    fetchImpl: async () => ({ ok: true, json: async () => manifest })
  });

  await until(() => provided !== null);
  assert.equal(calls.fetched.length, 1);
  assert.equal(calls.fetched[0].url, "/api/tools");
  assert.equal(provided.tools.length, 1);

  const tool = provided.tools[0];
  assert.equal(tool.name, "slugify");
  assert.equal(tool.description, "Turn text into a URL slug");
  // The shim runs in a separate vm realm, so compare serialised shapes.
  assert.equal(JSON.stringify(tool.inputSchema), JSON.stringify(manifest.tools[0].inputSchema));
  assert.equal(typeof tool.execute, "function");
});

await test("execute posts JSON and returns the rendered text", async () => {
  let provided = null;
  const { calls } = load({
    modelContext: { provideContext: (ctx) => { provided = ctx; } },
    fetchImpl: async (url) => {
      if (url === "/api/tools") return { ok: true, json: async () => manifest };
      return {
        ok: true,
        json: async () => okBody("Slug", [{ kind: "code", value: "hello-world" }])
      };
    }
  });

  await until(() => provided !== null);
  const out = await provided.tools[0].execute({ text: "Hello, World!" });

  assert.equal(JSON.stringify(out), JSON.stringify({ content: [{ type: "text", text: "hello-world" }] }));
  const post = calls.fetched.find((f) => f.url === "/api/tools/slugify");
  assert.ok(post, "execute did not POST to the tool endpoint");
  assert.equal(post.opts.method, "POST");
  assert.equal(post.opts.headers["content-type"], "application/json");
  assert.deepEqual(JSON.parse(post.opts.body), { text: "Hello, World!" });
});

await test("key/value lines and warnings are included in the text", async () => {
  let provided = null;
  load({
    modelContext: { provideContext: (ctx) => { provided = ctx; } },
    fetchImpl: async (url) =>
      url === "/api/tools"
        ? { ok: true, json: async () => manifest }
        : {
            ok: true,
            json: async () => okBody("Rate", [{ kind: "key_value", key: "Annual", value: "£126,500.00" }], ["rates from 2026-11-13"])
          }
  });

  await until(() => provided !== null);
  const out = await provided.tools[0].execute({});
  assert.equal(out.content[0].text, "Annual: £126,500.00\nrates from 2026-11-13");
});

await test("a bad request returns isError, it does not throw", async () => {
  let provided = null;
  load({
    modelContext: { provideContext: (ctx) => { provided = ctx; } },
    fetchImpl: async (url) =>
      url === "/api/tools"
        ? { ok: true, json: async () => manifest }
        : { ok: false, json: async () => ({ error: { message: "Text is required." } }) }
  });

  await until(() => provided !== null);
  const out = await provided.tools[0].execute({});
  assert.equal(out.isError, true);
  assert.equal(out.content[0].text, "Text is required.");
});

await test("a network failure returns isError", async () => {
  let provided = null;
  load({
    modelContext: { provideContext: (ctx) => { provided = ctx; } },
    fetchImpl: async (url) => {
      if (url === "/api/tools") return { ok: true, json: async () => manifest };
      throw new Error("network down");
    }
  });

  await until(() => provided !== null);
  const out = await provided.tools[0].execute({ text: "x" });
  assert.equal(out.isError, true);
  assert.match(out.content[0].text, /network down/);
});

await test("a throwing provideContext does not break registration of the page", async () => {
  const { calls } = load({
    modelContext: {
      provideContext: () => {
        throw new Error("browser said no");
      }
    },
    fetchImpl: async () => ({ ok: true, json: async () => manifest })
  });
  await until(() => calls.debug.length > 0);
  assert.match(calls.debug.join(" "), /registration failed/);
});

await test("on a /lab page, execute uses the gated API prefix", async () => {
  let provided = null;
  const { calls } = load({
    pathname: "/lab/slugify",
    modelContext: { provideContext: (ctx) => { provided = ctx; } },
    fetchImpl: async (url) => {
      if (url === "/api/tools") return { ok: true, json: async () => manifest };
      return { ok: true, json: async () => okBody("Slug", [{ kind: "code", value: "x" }]) };
    }
  });

  await until(() => provided !== null);
  await provided.tools[0].execute({ text: "x" });
  assert.ok(calls.fetched.some((f) => f.url === "/api/lab/slugify"));
  assert.ok(!calls.fetched.some((f) => f.url === "/api/tools/slugify"));
});

console.log(process.exitCode ? "\nJS tests failed" : "\nJS tests passed");
