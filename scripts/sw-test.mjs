// Tests the service worker's offline logic with a fake network and cache:
//   node scripts/sw-test.mjs
import { readFileSync } from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";

const src = readFileSync(new URL("../internal/web/ui/assets/sw.js", import.meta.url), "utf8");
const store = new Map();
const handlers = {};
let network; // (req) => Promise<Response>
// in a worker, relative URLs resolve against its origin
class SWRequest extends Request {
  constructor(input, init) { super(typeof input === "string" ? new URL(input, "https://n.example") : input, init); }
}
const sandbox = {
  self: { addEventListener: (t, f) => (handlers[t] = f), skipWaiting() {}, clients: { claim() {} } },
  location: { origin: "https://n.example" },
  navigator: { onLine: true },
  caches: {
    open: async () => ({ put: async (k, r) => store.set(k.url ?? k, await r.clone().text()), match: async (k) => store.has(k.url ?? k) ? new Response(store.get(k.url ?? k)) : undefined }),
    match: async (k) => (store.has(k.url ?? k) ? new Response(store.get(k.url ?? k)) : undefined),
    keys: async () => [], delete: async () => true,
  },
  fetch: (req) => network(req),
  Request: SWRequest, Response, Headers, URL, setTimeout, Promise, encodeURIComponent,
};
vm.runInNewContext(src, sandbox);

async function call(body, { timeoutCheck } = {}) {
  let answer;
  const req = new SWRequest("/api/tools/cis_grades", { method: "POST", body });
  handlers.fetch({ request: req, respondWith: (p) => (answer = p) });
  return answer;
}
const json = (o) => new Response(JSON.stringify(o));
const hang = () => new Promise(() => {});
const t0 = Date.now();

// 1. online: answer comes from the network and is cached
network = async () => json({ result: "fresh" });
assert.equal(await (await call("{}")).text(), '{"result":"fresh"}');

// 2. a hanging connection: after the patience the cached answer, marked offline
network = hang;
let res = await call("{}");
assert.equal(res.headers.get("X-Naknak-Offline"), "1");
assert.equal(await res.text(), '{"result":"fresh"}');
const waited = Date.now() - t0;
assert.ok(waited >= 5500 && waited < 9000, `waited ${waited} ms`);

// 3. the device knows it is offline: no waiting at all
sandbox.navigator.onLine = false;
const t1 = Date.now();
res = await call("{}");
assert.equal(res.headers.get("X-Naknak-Offline"), "1");
assert.ok(Date.now() - t1 < 500, "offline device must not wait");
sandbox.navigator.onLine = true;

// 4. a failing network without cache: an error, not a hang
network = async () => { throw new TypeError("Failed to fetch"); };
await assert.rejects(call('{"other":1}'));

// 5. a binding action never gets an old answer
network = hang;
const confirm = call('{"confirm":true}');
const raced = await Promise.race([confirm, new Promise((r) => setTimeout(() => r("still waiting"), 7000))]);
assert.equal(raced, "still waiting");

console.log("sw-test: ok");
process.exit(0);
