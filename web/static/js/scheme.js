(function () {
  "use strict";
  var KEY = "scheme";
  var root = document.documentElement;

  function apply(value) {
    root.setAttribute("data-scheme", value);
  }

  var saved = null;
  try { saved = localStorage.getItem(KEY); } catch (e) { /* private mode */ }
  if (saved === "dark" || saved === "light") {
    apply(saved);
  } else {
    var prefersDark = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
    apply(prefersDark ? "dark" : "light");
  }

  document.addEventListener("DOMContentLoaded", function () {
    var btn = document.getElementById("scheme-toggle");
    if (!btn) return;
    btn.addEventListener("click", function () {
      var next = root.getAttribute("data-scheme") === "dark" ? "light" : "dark";
      apply(next);
      try { localStorage.setItem(KEY, next); } catch (e) { /* ignore */ }
    });
  });
})();
