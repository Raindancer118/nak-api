// On the login page nobody is signed in: drop data cached for offline use.
if (window.caches) caches.delete("naknak-data-v1");

// The start-up log prints /login#token=…; the fragment never leaves the browser.
const m = location.hash.match(/token=([^&]+)/);
if (m) {
  history.replaceState(null, "", location.pathname);
  document.querySelector("details.alt").open = true;
  const form = document.querySelector(".token-form");
  form.token.value = decodeURIComponent(m[1]);
  form.requestSubmit();
}

for (const form of document.querySelectorAll("form")) {
  form.addEventListener("submit", () => {
    // the CIS check takes a few seconds: show it, and no double submits
    const b = form.querySelector("button[type=submit]");
    b.classList.add("busy");
    b.disabled = true;
  });
}

const toggle = document.querySelector(".pw-toggle");
toggle?.addEventListener("click", () => {
  const input = document.getElementById("password");
  const show = input.type === "password";
  input.type = show ? "text" : "password";
  toggle.textContent = show ? "verbergen" : "zeigen";
  toggle.setAttribute("aria-pressed", String(show));
  toggle.setAttribute("aria-label", show ? "Passwort verbergen" : "Passwort anzeigen");
  input.focus();
});
