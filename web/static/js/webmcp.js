// WebMCP bridge (FR-5). One file, no per-tool code: the manifest is the tool
// list, and every execute() posts to the same JSON endpoint the page uses.
//
// Contract:
//   - no navigator.modelContext  -> one debug line, no fetch, page unaffected
//   - provideContext replaces the whole set, so it is called once per load
//   - execute() never throws: failures come back as { isError: true }
//   - text is the result lines joined by newlines (the text content of the
//     rendered result), so the agent sees what the page shows
(function () {
  "use strict";

  var mc = navigator.modelContext;
  if (!mc || typeof mc.provideContext !== "function") {
    if (typeof console !== "undefined" && console.debug) {
      console.debug("WebMCP unavailable: tools still work from the page.");
    }
    return;
  }

  // A gated page talks to the gated API prefix (FR-9); the same script serves both.
  var path = (typeof location !== "undefined" && location.pathname) || "/";
  var prefix = path === "/lab" || path.indexOf("/lab/") === 0 ? "/api/lab/" : "/api/tools/";

  function textOf(body) {
    if (!body) return "";
    if (body.error) return body.error.message || "Request failed";

    var parts = [];
    var lines = body.lines || [];
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i];
      if (line.kind === "key_value") {
        parts.push(line.key + ": " + line.value);
      } else {
        parts.push(line.value);
      }
    }
    var warnings = body.warnings || [];
    for (var j = 0; j < warnings.length; j++) {
      parts.push(warnings[j]);
    }
    if (!parts.length) parts.push(body.title || "");
    return parts.join("\n").replace(/\n+$/, "");
  }

  function makeExecute(name) {
    return function (args) {
      return fetch(prefix + encodeURIComponent(name), {
        method: "POST",
        headers: { "content-type": "application/json", accept: "application/json" },
        body: JSON.stringify(args || {})
      })
        .then(function (res) {
          return res.json().then(function (body) {
            return { ok: res.ok, body: body };
          });
        })
        .then(function (out) {
          if (!out.ok || (out.body && out.body.error)) {
            return {
              isError: true,
              content: [{ type: "text", text: textOf(out.body) || "Request failed" }]
            };
          }
          return { content: [{ type: "text", text: textOf(out.body) }] };
        })
        .catch(function (err) {
          return {
            isError: true,
            content: [{ type: "text", text: String((err && err.message) || err) }]
          };
        });
    };
  }

  fetch("/api/tools", { headers: { accept: "application/json" } })
    .then(function (res) { return res.json(); })
    .then(function (manifest) {
      var tools = (manifest.tools || []).map(function (t) {
        return {
          name: t.name,
          description: t.description,
          inputSchema: t.inputSchema,
          execute: makeExecute(t.name)
        };
      });
      if (!tools.length) return;
      try {
        mc.provideContext({ tools: tools });
      } catch (err) {
        // Registration failing must never break the page (FR-5.4).
        if (typeof console !== "undefined" && console.debug) {
          console.debug("WebMCP registration failed:", err);
        }
      }
    })
    .catch(function () {
      // Manifest unavailable: the page is still fully usable.
    });
})();
