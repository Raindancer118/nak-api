// Runs before first paint so a stored theme does not flash.
const t = localStorage.getItem("nak-theme");
if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
