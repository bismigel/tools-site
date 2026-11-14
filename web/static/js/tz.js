// Progressive enhancement: make City 1 (the anchor) default to the visitor's own
// time zone. The server cannot know it; without JavaScript the tool keeps its
// sensible default. An explicit ?city1= in the URL always wins, so a shared link
// shows what the sender saw.
(function () {
  "use strict";

  var form = document.querySelector("form[data-tz-detect]");
  if (!form || !window.Intl || !Intl.DateTimeFormat) return;

  var name = form.getAttribute("data-tz-detect");
  var input = form.querySelector('[name="' + name + '"]');
  if (!input) return;

  // The reader already chose a city (URL or an edit before this ran).
  if (new URL(location.href).searchParams.has(name)) return;
  if (input.dataset.tzTouched === "1") return;

  var zone = "";
  try {
    zone = Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch (e) {
    return;
  }
  // "UTC" is what a locked-down browser reports; it is not a place.
  if (!zone || zone === "UTC") return;

  input.value = zone;
  input.dataset.tzTouched = "1";

  if (form.requestSubmit) {
    form.requestSubmit();
  } else if (window.htmx) {
    window.htmx.trigger(form, "submit");
  }
})();
