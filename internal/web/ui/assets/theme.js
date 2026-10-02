// Runs before first paint so a stored theme does not flash.
// ?theme=light|dark overrides it for one load, ?still switches motion off
// (both for screenshots and sharing a look).
const params = new URLSearchParams(location.search);
const t = params.get("theme") || localStorage.getItem("nak-theme");
if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
if (params.has("still")) document.documentElement.classList.add("still");
if (localStorage.getItem("nak-private") === "1") document.documentElement.classList.add("private");
