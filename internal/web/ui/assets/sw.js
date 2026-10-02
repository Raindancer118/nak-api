// naknak service worker: the app shell works offline;
// data always comes from the network first and only falls back to the last
// answer when there is no connection (marked with X-Naknak-Offline).
const SHELL = "naknak-shell-v4";
const DATA = "naknak-data-v1";
const SHELL_FILES = ["/assets/app.css", "/assets/app.js", "/assets/theme.js", "/assets/naknak.svg", "/assets/fonts/space-grotesk.woff2", "/manifest.webmanifest"];

self.addEventListener("install", (e) => {
  e.waitUntil(caches.open(SHELL).then((c) => c.addAll(SHELL_FILES)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (e) => {
  e.waitUntil(caches.keys()
    .then((keys) => Promise.all(keys.filter((k) => k !== SHELL && k !== DATA).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});

// logout: nothing personal stays on the device
self.addEventListener("message", (e) => {
  if (e.data === "clear") e.waitUntil(caches.delete(DATA));
});

self.addEventListener("fetch", (e) => {
  const req = e.request;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;

  if (req.method === "POST" && url.pathname.startsWith("/api/tools/")) {
    e.respondWith(toolCall(req, url));
    return;
  }
  if (req.method !== "GET") return;
  if (url.pathname.startsWith("/assets/") || url.pathname === "/manifest.webmanifest") {
    e.respondWith(networkFirst(req));
    return;
  }
  if (req.mode === "navigate" && url.pathname === "/") {
    const net = fetch(req).then((res) => {
      // clone now: once the page has the response its body is gone
      if (res.ok && !res.redirected) {
        const copy = res.clone();
        caches.open(SHELL).then((c) => c.put("/", copy));
      }
      return res;
    });
    e.respondWith(patient(net, async () => (await caches.match("/")) || offlinePage(), 5000));
  }
  // /files, /api/events, /calendar, /login: network only
});

// The shell comes from the network whenever there is one (an update shows
// up on the next load, not the one after); the cache is only the offline copy.
async function networkFirst(req) {
  const cache = await caches.open(SHELL);
  try {
    const res = await fetch(req);
    if (res.ok) cache.put(req, res.clone());
    return res;
  } catch (err) {
    const hit = await cache.match(req);
    if (hit) return hit;
    throw err;
  }
}

// No copy of the portal yet and no connection: a page of our own instead of
// the browser's error (an app WebView would show its bare error screen).
function offlinePage() {
  return new Response(`<!doctype html><html lang="de"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>naknak · offline</title><style>
:root{color-scheme:light dark;--bg:#f6f3ec;--ink:#0d1b33;--dim:#5b6780}@media(prefers-color-scheme:dark){:root{--bg:#07101f;--ink:#f2f5fb;--dim:#9aa6bd}}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--bg);color:var(--ink);font:16px/1.5 system-ui,sans-serif;text-align:center;padding:2rem;box-sizing:border-box}
img{width:88px;height:88px}h1{font-size:1.4rem;margin:.6rem 0 .3rem}p{color:var(--dim);max-width:24rem;margin:0 auto 1.2rem}
button{font:inherit;font-weight:600;padding:.75rem 1.4rem;border-radius:12px;border:0;background:var(--ink);color:var(--bg);min-height:48px}
</style></head><body><main><img src="/assets/naknak.svg" alt=""><h1>Keine Verbindung</h1>
<p>naknak ist gerade nicht erreichbar, und auf diesem Gerät ist noch kein Stand gespeichert. Ist das Portal einmal geladen, geht es auch offline.</p>
<button onclick="location.reload()">Erneut versuchen</button></main></body></html>`,
    { status: 503, headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" } });
}

// A bad connection rarely fails fast: requests just hang. After `wait` ms
// (at once when the device knows it is offline) the cached answer goes out;
// the network answer still lands in the cache when it arrives.
function patient(net, cached, wait) {
  return new Promise((resolve, reject) => {
    let done = false;
    const fallback = async (err) => {
      if (done) return;
      const hit = await cached();
      if (hit && !done) { done = true; resolve(hit); }
      else if (err && !done) { done = true; reject(err); }
    };
    net.then((res) => { if (!done) { done = true; resolve(res); } }, fallback);
    setTimeout(fallback, navigator.onLine === false ? 0 : wait);
  });
}

async function toolCall(req, url) {
  const body = await req.clone().text();
  // one cache entry per tool + arguments; ?fresh/?wait do not change the data
  const key = new Request(`/__offline${url.pathname}?b=${encodeURIComponent(body)}`);
  const net = fetch(req).then(async (res) => {
    if (res.ok) (await caches.open(DATA)).put(key, res.clone());
    return res;
  });
  // a binding action never gets an old answer
  if (/"confirm"\s*:\s*true/.test(body)) return net;
  return patient(net, async () => {
    const hit = await caches.match(key);
    if (!hit) return null;
    const headers = new Headers(hit.headers);
    headers.set("X-Naknak-Offline", "1");
    return new Response(await hit.blob(), { status: 200, headers });
  }, 6000);
}
