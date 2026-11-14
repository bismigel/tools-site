// Dev-only helpers (NFR-8.3). Served and referenced only when -dev is on.
(function () {
  "use strict";

  // Console poking without a WebMCP-capable browser:
  //   await __tools.list()
  //   await __tools.call("slugify", { text: "Hello, World!" })
  window.__tools = {
    list: function () {
      return fetch("/api/tools", { headers: { accept: "application/json" } }).then(function (r) {
        return r.json();
      });
    },
    call: function (name, args) {
      return fetch("/api/tools/" + encodeURIComponent(name), {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(args || {})
      }).then(function (r) { return r.json(); });
    }
  };

  // Reload when a template changes on disk (server-side mtime watcher).
  try {
    var es = new EventSource("/dev/reload");
    es.addEventListener("reload", function () { location.reload(); });
  } catch (e) { /* no SSE, no problem */ }
})();
