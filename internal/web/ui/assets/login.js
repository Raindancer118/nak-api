// The start-up log prints /login#token=…; the fragment never leaves the browser.
const m = location.hash.match(/token=([^&]+)/);
if (m) {
  history.replaceState(null, "", location.pathname);
  const form = document.querySelector("form");
  form.token.value = decodeURIComponent(m[1]);
  form.requestSubmit();
}
