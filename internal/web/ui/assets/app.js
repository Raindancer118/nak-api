// naknak web UI. Vanilla ES module; all data goes into the DOM via textContent,
// never as HTML, and inline styles are avoided (CSP) — custom properties are
// set through the CSSOM instead.

const $ = (sel, root = document) => root.querySelector(sel);
const reduced = matchMedia("(prefers-reduced-motion: reduce)");

// ── DOM helper ──────────────────────────────────────────────────────────────

function h(tag, props = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (v == null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k === "text") el.textContent = v;
    else if (k === "vars") for (const [n, x] of Object.entries(v)) el.style.setProperty(n, x);
    else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "href") el.setAttribute("href", safeHref(v));
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const kid of kids.flat(Infinity)) {
    if (kid == null || kid === false) continue;
    el.append(kid instanceof Node ? kid : document.createTextNode(String(kid)));
  }
  return el;
}

// Only in-app links (#…, /path) and https URLs; "//host" is protocol-relative
// and would leave the site, "/\\host" is treated the same way by browsers.
function safeHref(v) {
  const s = String(v ?? "").trim();
  if (s.startsWith("#")) return s;
  if (s.startsWith("/") && !s.startsWith("//") && !s.startsWith("/\\")) return s;
  if (/^https:\/\/[^/\\]/i.test(s)) return s;
  return "#";
}

const svg = (paths) => {
  const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  s.setAttribute("viewBox", "0 0 24 24");
  s.setAttribute("aria-hidden", "true");
  for (const d of paths) {
    const p = document.createElementNS("http://www.w3.org/2000/svg", "path");
    p.setAttribute("d", d);
    s.append(p);
  }
  return s;
};

const icons = {
  refresh: ["M20 11a8 8 0 1 0-2.3 5.7", "M20 4v7h-7"],
  system: ["M4 5h16v11H4z", "M9 20h6", "M12 16v4"],
  light: ["M12 4V2", "M12 22v-2", "M4.9 4.9 3.5 3.5", "M20.5 20.5l-1.4-1.4", "M4 12H2", "M22 12h-2", "M4.9 19.1l-1.4 1.4", "M20.5 3.5l-1.4 1.4", "M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z"],
  dark: ["M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z"],
  back: ["M15 18l-6-6 6-6"],
  gear: ["M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z", "M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"],
};

// ── API ─────────────────────────────────────────────────────────────────────

let fresh = false;
let newest = null;
// Calls of the current page that are still running, and the ones the server
// answered with stale data — those are revalidated once the page is drawn.
const pending = new Set();
const staleSeen = new Map();

class ToolError extends Error {
  constructor(msg, body) {
    super(msg);
    this.body = body || {};
  }
}

async function post(tool, args, query = "", headers = {}) {
  const res = await fetch(`/api/tools/${tool}${query}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...headers },
    body: JSON.stringify(args),
    credentials: "same-origin",
  });
  if (res.status === 401) {
    location.href = "/login";
    throw new ToolError("nicht angemeldet");
  }
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ToolError(body.error || `HTTP ${res.status}`, body);
  return body;
}

function api(tool, args = {}, { wait = false } = {}) {
  const p = post(tool, args, fresh ? "?fresh=1" : wait ? "?wait=1" : "").then((body) => {
    const at = new Date(body.fetched_at);
    if (!newest || at < newest) newest = at; // the oldest data on screen decides
    stamp();
    if (body.stale && !wait) staleSeen.set(tool + JSON.stringify(args), { tool, args, text: JSON.stringify(body.result) });
    return body.result;
  });
  if (!wait) {
    pending.add(p);
    const done = () => pending.delete(p);
    p.then(done, done);
  }
  return p;
}

async function settle() {
  while (pending.size) await Promise.allSettled([...pending]);
}

// ── dates ───────────────────────────────────────────────────────────────────

// CIS and the cross tools use "Fr 02.10.2026 10:40" / "02.10.2026" / "02.10.2026 10:40".
function parseDE(s) {
  const m = String(s || "").match(/(\d{2})\.(\d{2})\.(\d{4})(?:\s+(\d{1,2}):(\d{2}))?/);
  if (!m) return null;
  return new Date(+m[3], +m[2] - 1, +m[1], +(m[4] || 0), +(m[5] || 0));
}

const fmtDay = new Intl.DateTimeFormat("de-DE", { weekday: "long", day: "numeric", month: "long" });
const fmtShort = new Intl.DateTimeFormat("de-DE", { weekday: "short", day: "2-digit", month: "2-digit" });
const fmtTime = new Intl.DateTimeFormat("de-DE", { hour: "2-digit", minute: "2-digit" });
const wd = ["So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"];

const startOfDay = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
const dayDiff = (d, now = new Date()) => Math.round((startOfDay(d) - startOfDay(now)) / 864e5);

function relDay(n) {
  if (n < 0) return "überfällig";
  if (n === 0) return "heute";
  if (n === 1) return "morgen";
  return `in ${n} T.`;
}

function greeting(now = new Date()) {
  const hr = now.getHours();
  if (hr < 5) return "Gute Nacht";
  if (hr < 11) return "Guten Morgen";
  if (hr < 18) return "Hallo";
  return "Guten Abend";
}

const isoDate = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

// ── shared bits ─────────────────────────────────────────────────────────────

const srcOf = (s) => (String(s || "").startsWith("moodle") ? "moodle" : "cis");
const srcName = { cis: "CIS", moodle: "Moodle" };

function tile(title, { cls = "", link, i = 0 } = {}, ...body) {
  const t = h("section", { class: `tile ${cls}`, vars: { "--i": i } },
    h("div", { class: "tile-head" }, h("h2", { text: title }), link && h("a", { href: link[1], text: link[0] })),
    ...body);
  return t;
}

const skeleton = () => [h("div", { class: "skel big-s" }), h("div", { class: "skel w80" }), h("div", { class: "skel w60" })];

function errorBox(err) {
  const box = h("div", { class: "err", role: "status" }, err.message);
  if (err.body?.drift && err.body.report_url) {
    box.append(" ", h("a", { href: err.body.report_url, target: "_blank", rel: "noopener", text: "Änderung melden" }));
  }
  return box;
}

// fill replaces a tile's skeleton once its data has arrived.
async function fill(tileEl, load, render) {
  const body = h("div", {}, skeleton());
  tileEl.append(body);
  try {
    const data = await load();
    body.replaceChildren(...[render(data)].flat().filter(Boolean));
  } catch (err) {
    body.replaceChildren(errorBox(err));
  }
}

function count(n, unit) {
  return h("span", { class: "big" }, h("span", { class: "count", vars: { "--n": Math.round(n) } }), unit && h("span", { class: "unit", text: unit }));
}

// decimal numbers (grade averages) count up in JS; integers use the CSS counter
function decimal(n, digits = 1) {
  const el = h("span", { class: "big", text: (0).toFixed(digits).replace(".", ",") });
  const target = Number(n) || 0;
  if (reduced.matches) {
    el.textContent = target.toFixed(digits).replace(".", ",");
    return el;
  }
  const t0 = performance.now();
  const step = (t) => {
    const k = Math.min(1, (t - t0) / 1100);
    const e = 1 - Math.pow(1 - k, 4);
    el.textContent = (target * e).toFixed(digits).replace(".", ",");
    if (k < 1) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
  return el;
}

function bar(p, color) {
  return h("div", { class: "progress" }, h("i", { vars: { "--p": Math.max(0, Math.min(1, p)), ...(color ? { "--c": color } : {}) } }));
}

function emptyRow(text) {
  return h("p", { class: "empty", text });
}

// ── overview ────────────────────────────────────────────────────────────────

function nextUp(agenda, dash) {
  const now = new Date();
  const items = [];
  for (const d of agenda?.days || []) {
    for (const e of d.events || []) {
      const start = parseDE(`${e.date} ${e.start}`);
      const end = parseDE(`${e.date} ${e.end}`) || start;
      if (start && end >= now) items.push({ start, end, title: e.title, nr: e.module_nr, room: e.room, kind: e.kind, src: srcOf(e.source) });
    }
  }
  for (const x of dash?.registered_exams || []) {
    const start = parseDE(x.start);
    if (start && start >= now) items.push({ start, end: start, title: `Klausur ${x.module}`, kind: "Klausur", src: "cis" });
  }
  items.sort((a, b) => a.start - b.start);
  return items;
}

function renderNext(items) {
  const [item, ...rest] = items;
  if (!item) {
    return h("div", { class: "next" },
      h("div", {}, h("div", { class: "what", text: "Nichts geplant." }), h("div", { class: "meta", text: "In den nächsten sieben Tagen stehen keine Termine im Stundenplan." })));
  }
  const now = new Date();
  const running = item.start <= now && item.end > now;
  const mins = Math.round(((running ? item.end : item.start) - now) / 6e4);
  let n = mins, unit = "min";
  if (mins >= 48 * 60) { n = dayDiff(item.start, now); unit = n === 1 ? "Tag" : "Tage"; }
  else if (mins >= 90) { n = Math.round(mins / 60); unit = "Std"; }
  const when = running
    ? `läuft seit ${fmtTime.format(item.start)}`
    : `${dayDiff(item.start) === 0 ? "heute" : fmtShort.format(item.start)} · ${fmtTime.format(item.start)}${item.end > item.start ? `–${fmtTime.format(item.end)}` : ""}`;
  return h("div", { class: "next", "data-src": item.src },
    h("div", {},
      h("span", { class: `chip ${item.src}`, text: item.kind || srcName[item.src] }),
      h("a", { class: "what", href: item.nr ? `#/modul/${firstNr(item.nr)}` : "#/woche", text: item.title }),
      h("div", { class: "meta" }, h("span", { text: when }), item.room && h("span", { text: item.room }), item.nr && h("span", { text: item.nr }))),
    h("div", { class: "countdown" },
      h("h2", { text: running ? "noch" : "in" }),
      count(n, unit)),
    running && bar((now - item.start) / (item.end - item.start)),
    rest.length > 0 && h("div", { class: "after" },
      h("h2", { text: "Danach" }),
      h("ul", { class: "rows" }, rest.slice(0, 2).map((x, j) => h("li", {}, h("div", { class: "row", vars: { "--j": j + 2 } },
        h("span", { class: "dot", "data-src": x.src }),
        h("span", { class: "t" }, x.title, h("span", { class: "s", text: [`${dayDiff(x.start) === 0 ? "heute" : fmtShort.format(x.start)} · ${fmtTime.format(x.start)}`, x.room].filter(Boolean).join(" · ") })),
        h("span", { class: `chip ${x.src}`, text: x.kind || srcName[x.src] })))))));
}

function deadlineRows(list, limit = 6) {
  if (!list.length) return emptyRow("Keine Fristen in den nächsten 30 Tagen.");
  return h("ul", { class: "rows" }, list.slice(0, limit).map((d, j) => {
    const src = srcOf(d.source);
    const due = d.urgent || d.overdue;
    const kids = [
      h("span", { class: "dot", "data-src": src, title: srcName[src] }),
      h("span", { class: "t" }, d.what, h("span", { class: "s", text: [d.when, d.course].filter(Boolean).join(" · ") })),
      h("span", { class: `chip ${due ? "due" : ""}`, text: relDay(d.in_days) }),
    ];
    const props = { class: "row", vars: { "--j": j } };
    const href = src === "moodle" ? "#/abgaben" : d.module_nr ? `#/modul/${firstNr(d.module_nr)}` : null;
    return h("li", {}, href ? h("a", { ...props, href }, kids) : h("div", props, kids));
  }));
}

function weekStrip(agenda) {
  const byDay = new Map();
  for (const d of agenda?.days || []) {
    const day = parseDE(d.date);
    if (day) byDay.set(isoDate(day), d.events || []);
  }
  const today = startOfDay(new Date());
  const cells = [];
  for (let j = 0; j < 7; j++) {
    const day = new Date(today.getFullYear(), today.getMonth(), today.getDate() + j);
    const evs = byDay.get(isoDate(day)) || [];
    cells.push(h("a", { class: `d ${j === 0 ? "today" : ""}`, href: "#/woche", vars: { "--j": j }, title: `${evs.length} Termine` },
      h("span", { class: "dn" }, wd[day.getDay()], h("b", { text: day.getDate() })),
      evs.slice(0, 5).map((e) => h("span", { class: "ev", "data-src": srcOf(e.source) }))));
  }
  const n = [...byDay.values()].reduce((s, e) => s + e.length, 0);
  return [h("div", { class: "strip" }, cells), h("p", { class: "empty", text: n ? `${n} Termine in 7 Tagen` : "Diese Woche ist frei." })];
}

function credits(progress, g) {
  const c = progress?.credits;
  if (c?.total_required) return { earned: c.total_earned || 0, total: c.total_required };
  return { earned: g?.stats?.credits_earned || 0, total: 0 };
}

function creditMeter(progress, g) {
  const { earned, total } = credits(progress, g);
  return h("div", { class: "meter" },
    h("div", { class: "meter-label" }, h("span", { text: total ? `${earned} von ${total} Credits` : `${earned} Credits` }), total && h("span", { text: `${Math.round((earned / total) * 100)} %` })),
    total && bar(earned / total, "var(--ok)"));
}

function gradeSummary([g, progress]) {
  const st = g.stats || {};
  const recent = [...(g.overview?.modules || [])]
    .filter((m) => m.grade_value)
    .sort((a, b) => (parseDE(b.entry_date || b.exam_date) || 0) - (parseDE(a.entry_date || a.exam_date) || 0))
    .slice(0, 3);
  return [
    h("div", { class: "avg" }, decimal(st.weighted_average, 1), h("span", { class: "empty", text: "Schnitt (gewichtet)" })),
    creditMeter(progress, g),
    h("ul", { class: "rows" }, recent.map((m, j) => h("li", {}, h("a", { class: "row", href: `#/modul/${firstNr(m.module_nr)}`, vars: { "--j": j } },
      h("span", { class: "dot", "data-src": "cis" }),
      h("span", { class: "t" }, m.title, h("span", { class: "s", text: m.entry_date || m.exam_date })),
      h("span", { class: `chip ${gradeClass(m)}`, text: m.grade }))))),
  ];
}

function examRows(dash) {
  const list = (dash.registered_exams || []).map((x) => ({ ...x, at: parseDE(x.start) })).sort((a, b) => (a.at || 0) - (b.at || 0));
  if (!list.length) return emptyRow("Zu keiner Prüfung angemeldet.");
  return h("ul", { class: "rows" }, list.map((x, j) => h("li", {}, h("div", { class: "row", vars: { "--j": j } },
    h("span", { class: "dot", "data-src": "cis" }),
    h("span", { class: "t" }, x.module, h("span", { class: "s", text: x.start })),
    x.at && h("span", { class: `chip ${dayDiff(x.at) <= 3 ? "due" : ""}`, text: relDay(dayDiff(x.at)) })))));
}

const courseTitle = (c) => String(c.name || c.shortname).replace(/^\s*[A-Z]{1,2}\d{3}\s*-\s*/i, "").split(" - ")[0];

function moodleSummary([dash, courses]) {
  const u = dash.moodle_unread || {};
  const list = courses || [];
  return h("div", { class: "band" },
    h("div", { class: "band-num" },
      h("div", { class: "avg" }, count((u.notifications || 0) + (u.conversations || 0)), h("span", { class: "empty", text: "ungelesen" })),
      h("p", { class: "empty", text: `${u.notifications || 0} Benachrichtigungen · ${u.conversations || 0} Nachrichten` }),
      h("div", { class: "go-links" }, h("a", { class: "go", href: "#/neu", text: "Neuigkeiten" }), h("a", { class: "go", href: "#/nachrichten", text: "Nachrichten" }))),
    h("ul", { class: "courses", "aria-label": `${list.length} aktuelle Kurse` }, list.map((c, j) => h("li", { vars: { "--j": j } },
      h("a", { href: `#/kurs/${c.id}`, title: c.name },
        h("span", { class: "nr", text: firstNr(c.name) || firstNr(c.shortname) }), courseTitle(c),
        c.progress && h("span", { class: "pct", text: c.progress }))))));
}

function firstName(full) {
  const s = String(full || "").trim();
  if (!s) return "";
  return s.includes(",") ? s.split(",")[1].trim().split(/\s+/)[0] : s.split(/\s+/)[0];
}

async function overview(root) {
  const now = new Date();
  const title = h("h1", {}, `${greeting(now)}.`, h("small", { text: fmtDay.format(now) }));
  const hero = h("section", { class: "hero" }, title);
  root.append(hero);
  api("moodle_whoami").then((me) => {
    const name = firstName(me.user);
    if (name) title.firstChild.textContent = `${greeting(now)}, ${name}.`;
  }).catch(() => {});
  hero.append(searchBox());
  const dash = api("nak_dashboard");
  const agenda = api("nak_agenda", { days: 7 });
  const grid = h("div", { class: "grid", id: "overview-grid" });
  root.append(grid);

  const tNext = tile("Als Nächstes", { cls: "w8", i: 0 });
  const tDead = tile("Fristen", { i: 1, link: ["Woche", "#/woche"] });
  const tWeek = tile("Diese Woche", { cls: "half", i: 2, link: ["Alle Termine", "#/woche"] });
  const tGrades = tile("Noten", { cls: "half", i: 3, link: ["Alle Noten", "#/noten"] });
  const tExams = tile("Prüfungen", { cls: "half", i: 4 });
  const tMoodle = tile("Moodle · aktuelle Kurse", { cls: "w12", i: 5, link: ["Alle Kurse", "#/kurse"] });
  grid.append(tNext, tDead, tWeek, tGrades, tExams, tMoodle);

  fill(tNext, () => Promise.all([agenda, dash]), ([a, d]) => renderNext(nextUp(a, d)));
  fill(tDead, () => api("nak_deadlines", { days: 30 }), (r) => deadlineRows(r.deadlines || []));
  fill(tWeek, () => agenda, weekStrip);
  fill(tGrades, () => Promise.all([api("cis_grades"), api("cis_progress").catch(() => null)]), gradeSummary);
  fill(tExams, () => dash, examRows);
  fill(tMoodle, () => Promise.all([dash, api("moodle_courses", { classification: "current" })]), moodleSummary);

  const foot = h("div", { class: "foot" });
  root.append(foot);
  dash.then((d) => {
    const q = d.quarter?.current;
    if (q) foot.append(h("span", {}, "Quartal ", h("b", { text: q.name }), ` · ${q.from} – ${q.to}`));
    const bal = d.balance && Object.entries(d.balance)[0];
    if (bal) foot.append(h("span", {}, `${bal[0]} `, h("b", { text: bal[1] })));
  }).catch(() => {});
}

// ── week ────────────────────────────────────────────────────────────────────

async function week(root) {
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Woche", h("small", { text: "Vorlesungen, Klausuren und Moodle-Fristen der nächsten 14 Tage" }))));
  const box = h("div", { class: "days" }, skeleton(), skeleton());
  root.append(box);
  try {
    const a = await api("nak_agenda", { days: 14 });
    const days = a.days || [];
    if (!days.length) {
      box.replaceChildren(emptyRow("In den nächsten 14 Tagen steht nichts an."));
      return;
    }
    box.replaceChildren(...days.map((d) => {
      const day = parseDE(d.date);
      const diff = day ? dayDiff(day) : 99;
      return h("section", { class: `day ${diff === 0 ? "today" : ""}` },
        h("h3", {}, day ? fmtDay.format(day).split(",")[0] : d.date, h("span", { text: day ? `${day.getDate()}.${day.getMonth() + 1}.` : "" })),
        h("ul", { class: "rows" }, (d.events || []).map((e) => {
          const src = srcOf(e.source);
          return h("li", {}, h("div", { class: "row" },
            h("span", { class: "time" }, e.start, e.end && e.end !== e.start && h("small", { text: e.end })),
            h("span", { class: "dot", "data-src": src }),
            h("span", { class: "t" }, e.title, h("span", { class: "s", text: [e.module_nr, e.room, e.lecturer].filter(Boolean).join(" · ") })),
            h("span", { class: `chip ${src}`, text: e.kind || srcName[src] })));
        })));
    }));
    if (a.unavailable_sources) box.prepend(errorBox(new Error(`Nicht erreichbar: ${Object.keys(a.unavailable_sources).join(", ")}`)));
  } catch (err) {
    box.replaceChildren(errorBox(err));
  }
}

// ── grades ──────────────────────────────────────────────────────────────────

function gradeClass(m) {
  const s = `${m.status || ""} ${m.grade || ""}`.toLowerCase();
  if (s.includes("nicht") || (m.grade_value && m.grade_value > 4)) return "bad";
  if ((m.grade_value && m.grade_value <= 4) || s.includes("bestanden")) return "ok";
  return "";
}

function gradeState(m) {
  const c = gradeClass(m);
  return c === "ok" ? "passed" : c === "bad" ? "failed" : "open";
}

async function gradesPage(root) {
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Noten", h("small", { text: "Leistungsübersicht aus dem CIS" }))));
  const grid = h("div", { class: "grid" });
  root.append(grid);
  const tSum = tile("Überblick", { cls: "w12", i: 0 });
  const tList = tile("Module", { cls: "w12", i: 1 });
  grid.append(tSum, tList);
  const data = api("cis_grades");

  fill(tSum, () => data, (g) => {
    const st = g.stats || {};
    const total = parseInt(g.overview?.credits_total, 10) || 180;
    return h("div", { class: "grid" },
      h("div", { class: "tile-inner half" }, h("div", { class: "avg" }, decimal(st.weighted_average, 2), h("span", { class: "empty", text: "gewichteter Schnitt" }))),
      h("div", { class: "tile-inner half" },
        h("div", { class: "meter-label" }, h("span", { text: `${st.credits_earned || 0} von ${total} Credits` }), h("span", { text: `${st.passed || 0} bestanden · ${st.open || 0} offen · ${st.failed || 0} nicht bestanden` })),
        bar((st.credits_earned || 0) / total, "var(--ok)")));
  });

  fill(tList, () => data, (g) => {
    const mods = g.overview?.modules || [];
    let filter = "all";
    const body = h("tbody");
    const draw = () => {
      const rows = mods.filter((m) => filter === "all" || gradeState(m) === filter);
      body.replaceChildren(...rows.map((m, j) => h("tr", { vars: { "--j": Math.min(j, 20) } },
        h("td", { class: "nr", text: m.module_nr }),
        h("td", { class: "name" }, h("a", { href: `#/modul/${firstNr(m.module_nr)}`, text: m.title })),
        h("td", { class: "nr", text: m.exam_date || "" }),
        h("td", { class: "nr r", text: m.attempt ? `${m.attempt}.` : "" }),
        h("td", { class: "nr r", text: m.credits || "" }),
        h("td", { class: "grade" }, h("span", { class: `chip ${gradeClass(m)}`, text: m.grade || m.status || "–" })))));
      if (!rows.length) body.append(h("tr", {}, h("td", { colspan: 6, class: "empty", text: "Keine Module in dieser Ansicht." })));
    };
    const filters = h("div", { class: "filters", role: "group", "aria-label": "Filter" },
      [["all", "Alle"], ["passed", "Bestanden"], ["open", "Offen"], ["failed", "Nicht bestanden"]].map(([k, label]) =>
        h("button", { type: "button", "aria-pressed": k === filter ? "true" : "false", text: label, onclick: (e) => {
          filter = k;
          for (const b of filters.children) b.setAttribute("aria-pressed", b === e.currentTarget ? "true" : "false");
          draw();
        } })));
    draw();
    return [filters, h("div", { class: "table-wrap" }, h("table", {},
      h("thead", {}, h("tr", {}, ["Nr.", "Modul", "Prüfung", "Versuch", "Credits", "Note"].map((x, i) => h("th", { class: i >= 3 ? "r" : "", text: x })))),
      body))];
  });
}

// ── modules ─────────────────────────────────────────────────────────────────

// ── search ──────────────────────────────────────────────────────────────────

function searchBox() {
  const input = h("input", { type: "search", class: "search-input", placeholder: "Suchen: Kurse, Module, Dateien, Aktivitäten …", "aria-label": "Suchen", autocomplete: "off", spellcheck: "false" });
  const list = h("div", { class: "search-results", role: "listbox", hidden: true });
  const wrap = h("div", { class: "search", role: "search" },
    h("span", { class: "search-icon" }, svg(["M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14z", "M20 20l-4-4"])),
    input, h("kbd", { text: "/" }), list);
  let timer, seq = 0, active = -1;

  function items() { return [...list.querySelectorAll(".hit")]; }
  function mark(i) {
    const all = items();
    active = Math.max(-1, Math.min(i, all.length - 1));
    all.forEach((el, j) => el.setAttribute("aria-selected", j === active ? "true" : "false"));
    all[active]?.scrollIntoView({ block: "nearest" });
  }
  function group(title, hits) {
    if (!hits.length) return null;
    return h("div", { class: "hit-group" }, h("h2", { text: title }), hits);
  }
  function hit(props, kids) {
    return h(props.href ? "a" : "button", { class: "hit", role: "option", ...props, ...(props.href ? {} : { type: "button" }) }, kids);
  }

  function unitHit(u) {
    const cur = u.courses.find((c) => c.state === "current");
    return hit({ href: `#/modul/${u.nr}` }, [
      h("span", { class: "dot", "data-src": cur ? "moodle" : "cis" }),
      h("span", { class: "t", text: u.title || u.nr }),
      h("span", { class: "s", text: [u.nr, unitLabel(u)].filter((x, i, a) => x && a.indexOf(x) === i).join(" · ") })]);
  }

  function contentHits(found) {
    return (found || []).slice(0, 10).map((x) => {
      // moodle_find returns either a matching file (file/fileurl) or a
      // matching activity (module/type, maybe with its files)
      const f = x.fileurl ? { name: x.file, fileurl: x.fileurl, size: x.size } : x.type === "resource" && x.files?.length === 1 ? x.files[0] : null;
      if (f) return hit({ onclick: () => openFile(f.fileurl) }, [h("span", { class: "dot", "data-src": "moodle" }), h("span", { class: "t", text: f.name || x.module }), h("span", { class: "s", text: [x.course, f.size].filter(Boolean).join(" · ") })]);
      return hit({ href: x.courseid ? `#/kurs/${x.courseid}` : "#/kurse" }, [h("span", { class: "dot", "data-src": "moodle" }), h("span", { class: "t", text: x.module }), h("span", { class: "s", text: [typeName[x.type] || x.type, x.course].filter(Boolean).join(" · ") })]);
    });
  }

  async function findContent(q, my, head) {
    list.replaceChildren(...head, h("p", { class: "empty hit-note", text: "Suche in Dateien und Aktivitäten …" }));
    let found = [];
    try {
      const r = await api("moodle_find", { query: q });
      found = Array.isArray(r) ? r : r?.results || r?.hits || [];
    } catch {}
    if (my !== seq) return;
    list.replaceChildren(...head, group("Dateien & Aktivitäten", contentHits(found)) || h("p", { class: "empty hit-note", text: `Nichts zu „${q}“ gefunden.` }));
    mark(-1);
  }

  async function run(q) {
    const my = ++seq;
    let units = [], orphans = [];
    try { ({ units, orphans } = await loadUnits()); } catch (e) { console.error("nak units", e); }
    if (my !== seq) return;
    const want = norm(q).join(" ");
    const uh = units
      .filter((u) => matches(q, norm([u.nr, ...u.aliases, u.title, ...u.courses.map((c) => c.name)].join(" "))))
      .sort((a, b) => (norm(b.title).join(" ") === want) - (norm(a.title).join(" ") === want)
        || b.courses.some((c) => c.state === "current") - a.courses.some((c) => c.state === "current"))
      .slice(0, 6);
    const oh = orphans.filter((c) => matches(q, norm(`${c.name} ${c.shortname}`))).slice(0, 4);
    list.hidden = false;
    if (!uh.length && !oh.length) {
      if (q.length >= 3) return findContent(q, my, []);
      list.replaceChildren(h("p", { class: "empty hit-note", text: "Weiter tippen …" }));
      return;
    }
    const head = [
      group("Module", uh.map(unitHit)),
      group("Weitere Kurse", oh.map((c) => hit({ href: `#/kurs/${c.id}` }, [h("span", { class: "dot", "data-src": "moodle" }), h("span", { class: "t", text: courseTitle(c) }), h("span", { class: "s", text: c.state === "current" ? "aktuell" : c.shortname })]))),
    ].filter(Boolean);
    const more = q.length >= 3 && hit({ class: "hit more", onclick: (e) => { e.stopPropagation(); findContent(q, ++seq, head); } }, [h("span", { class: "dot" }), h("span", { class: "t", text: `„${q}“ in Dateien und Aktivitäten suchen` }), h("span", { class: "s", text: "↵" })]);
    list.replaceChildren(...head, more || []);
    mark(uh.length + oh.length === 1 ? 0 : -1);
  }

  input.addEventListener("input", () => {
    clearTimeout(timer);
    const q = input.value.trim();
    if (!q) { list.hidden = true; seq++; return; }
    timer = setTimeout(() => run(q), 250);
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") { e.preventDefault(); mark(active + 1); }
    else if (e.key === "ArrowUp") { e.preventDefault(); mark(active - 1); }
    else if (e.key === "Enter" && active >= 0) { e.preventDefault(); items()[active].click(); }
    else if (e.key === "Escape") { list.hidden = true; input.blur(); }
  });
  list.addEventListener("click", () => { list.hidden = true; });
  document.addEventListener("click", (e) => { if (!wrap.contains(e.target)) list.hidden = true; });
  return wrap;
}

addEventListener("keydown", (e) => {
  if (e.key === "/" && !/^(INPUT|TEXTAREA)$/.test(document.activeElement?.tagName)) {
    const s = $(".search-input");
    if (s) { e.preventDefault(); s.focus(); }
  }
});

// ── files ───────────────────────────────────────────────────────────────────

// Opens a Moodle file inside nak: downloaded once on the server, then shown
// by the browser (PDF, images, text) or saved.
async function openFile(fileurl, btn) {
  const win = window.open("", "_blank"); // must open inside the click, before awaiting
  btn?.classList.add("busy");
  try {
    const r = await post("moodle_download", { fileurl });
    if (!r.download_url) throw new Error("Datei konnte nicht bereitgestellt werden");
    const u = `${r.download_url}?inline=1`;
    if (win) win.location = u;
    else location.href = u;
  } catch (err) {
    win?.close();
    alertBox(err.message);
  } finally {
    btn?.classList.remove("busy");
  }
}

function alertBox(msg) {
  const t = h("div", { class: "toast", role: "alert", text: msg });
  document.body.append(t);
  setTimeout(() => t.remove(), 5000);
}

function fileRow(f, j = 0) {
  const b = h("button", { class: "row file", type: "button", vars: { "--j": j }, onclick: (e) => openFile(f.fileurl, e.currentTarget) },
    h("span", { class: "ext", text: (String(f.name).split(".").pop() || "").slice(0, 4) }),
    h("span", { class: "t" }, f.name, h("span", { class: "s", text: [f.size, f.modified].filter(Boolean).join(" · ") })),
    h("span", { class: "chip moodle", text: "öffnen" }));
  return h("li", {}, b);
}

// ── courses ─────────────────────────────────────────────────────────────────

const typeName = {
  resource: "Datei", folder: "Ordner", url: "Link", page: "Seite", label: "Text", assign: "Abgabe", quiz: "Test",
  forum: "Forum", choice: "Abstimmung", feedback: "Umfrage", lesson: "Lektion", book: "Buch", h5pactivity: "H5P",
  bigbluebuttonbn: "Videokonferenz", scorm: "SCORM", wiki: "Wiki", glossary: "Glossar", workshop: "Workshop", data: "Datenbank",
};

async function coursesPage(root) {
  let which = "current";
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Kurse", h("small", { text: "Alle Moodle-Kurse, direkt hier lesen" }))));
  const box = h("div", { class: "mods" });
  const filters = h("div", { class: "filters", role: "group", "aria-label": "Zeitraum" },
    [["current", "Aktuell"], ["past", "Vergangen"], ["all", "Alle"]].map(([k, label]) =>
      h("button", { type: "button", "aria-pressed": k === which ? "true" : "false", text: label, onclick: (e) => {
        which = k;
        for (const b of filters.children) b.setAttribute("aria-pressed", b === e.currentTarget ? "true" : "false");
        load();
      } })));
  root.querySelector(".page-head").append(filters);
  root.append(box);
  async function load() {
    box.replaceChildren(...[0, 1, 2].map((i) => tile("", { i }, skeleton())));
    try {
      const list = await api("moodle_courses", { classification: which });
      if (!list.length) return box.replaceChildren(emptyRow("Keine Kurse in dieser Ansicht."));
      box.replaceChildren(...list.map((c, i) => {
        const pct = parseInt(c.progress, 10);
        const t = tile("", { i: Math.min(i, 12) },
          h("a", { class: "mod", href: `#/kurs/${c.id}` },
            h("span", { class: "nr", text: firstNr(c.name) || c.shortname }),
            h("span", { class: "name", text: courseTitle(c) }),
            h("span", { class: "tags" }, c.last_access && h("span", { class: "chip", text: `zuletzt ${c.last_access.slice(0, 10)}` })),
            !isNaN(pct) && bar(pct / 100, "var(--moodle)")));
        t.querySelector(".tile-head").remove();
        return t;
      }));
    } catch (err) {
      box.replaceChildren(errorBox(err));
    }
  }
  await load();
}

function textBlock(text) {
  return text ? h("p", { class: "prose", text }) : null;
}

function moduleItem(m, courseId, assignByCm, j) {
  const kind = typeName[m.type] || m.type;
  const dates = Object.entries(m.dates || {}).map(([k, v]) => `${k} ${v}`).join(" · ");
  if (m.type === "label") return h("li", { class: "label" }, textBlock(m.text));
  const files = m.files || [];
  let action = null;
  if (m.type === "assign" && assignByCm[m.cmid]) action = h("a", { class: "chip due", href: `#/abgabe/${assignByCm[m.cmid]}`, text: "Aufgabe" });
  else if (m.type === "forum") action = h("a", { class: "chip moodle", href: `#/kurs/${courseId}/forum`, text: "Forum" });
  else if (m.type === "url" && m.links?.[0]) action = h("a", { class: "chip moodle", href: m.links[0], target: "_blank", rel: "noopener", text: "Link ↗" });
  else if (!files.length && m.type !== "page" && m.url) action = h("a", { class: "chip", href: m.url, target: "_blank", rel: "noopener", title: "Diese Aktivität geht nur in Moodle selbst", text: "in Moodle ↗" });
  return h("li", { class: "item", vars: { "--j": j } },
    h("div", { class: "item-head" },
      h("span", { class: "kind", text: kind }),
      h("span", { class: "t", text: m.name }),
      action),
    dates && h("span", { class: "s", text: dates }),
    textBlock(m.text),
    files.length > 0 && h("ul", { class: "rows files" }, files.map((f, k) => fileRow(f, k))));
}

function assignmentRows(as) {
  if (!as.length) return emptyRow("Keine Abgaben.");
  const now = Date.now() / 1000;
  return h("ul", { class: "rows" }, as.map((a, j) => {
    const open = !a.due_unix || a.due_unix > now;
    const days = a.due_unix ? dayDiff(new Date(a.due_unix * 1000)) : null;
    return h("li", {}, h("a", { class: "row", href: `#/abgabe/${a.assignid}`, vars: { "--j": j } },
      h("span", { class: "dot", "data-src": "moodle" }),
      h("span", { class: "t" }, a.name, h("span", { class: "s", text: [a.course, a.due ? `fällig ${a.due}` : "ohne Frist"].filter(Boolean).join(" · ") })),
      h("span", { class: `chip ${open && days !== null && days <= 3 ? "due" : ""}`, text: open ? (days === null ? "offen" : relDay(days)) : "vorbei" })));
  }));
}

function quizRows(qs) {
  if (!qs.length) return emptyRow("Keine Tests.");
  return h("ul", { class: "rows" }, qs.map((q, j) => h("li", {}, h("a", { class: "row", href: q.url, target: "_blank", rel: "noopener", title: "Tests laufen in Moodle selbst", vars: { "--j": j } },
    h("span", { class: "dot", "data-src": "moodle" }),
    h("span", { class: "t" }, q.name, h("span", { class: "s", text: [q.opens && `ab ${q.opens}`, q.closes && `bis ${q.closes}`, q.attempts_allowed && `Versuche: ${q.attempts_allowed}`].filter(Boolean).join(" · ") })),
    h("span", { class: "chip", text: "in Moodle ↗" })))));
}

async function assignmentPage(root, id) {
  root.append(h("a", { class: "back", href: "#/abgaben" }, svg(icons.back), "Abgaben"));
  const head = h("div", { class: "page-head" }, h("h1", { text: "Aufgabe" }));
  root.append(head);
  const box = h("div", { class: "grid" }, tile("", { cls: "w12" }, skeleton()));
  root.append(box);
  try {
    const a = await api("moodle_assignment", { assignid: Number(id) });
    head.querySelector("h1").replaceChildren(a.name, h("small", { text: a.course }));
    const sub = a.submission || {};
    box.replaceChildren(
      tile("Aufgabe", { cls: "w8" },
        h("dl", { class: "kv" },
          a.opens && [h("dt", { text: "ab" }), h("dd", { text: a.opens })],
          h("dt", { text: "fällig" }), h("dd", { text: a.due || "ohne Frist" }),
          a.cutoff && [h("dt", { text: "letzte Abgabe" }), h("dd", { text: a.cutoff })],
          a.max_grade && [h("dt", { text: "max. Punkte" }), h("dd", { text: a.max_grade })]),
        textBlock(a.intro),
        (a.attachments || []).length > 0 && h("ul", { class: "rows files meter" }, a.attachments.map((f, k) => fileRow(f, k)))),
      tile("Meine Abgabe", { cls: "", i: 1 },
        h("dl", { class: "kv" }, Object.entries(sub).filter(([, v]) => typeof v !== "object" && v !== "").map(([k, v]) => [h("dt", { text: statusLabel[k] || k }), h("dd", { text: statusLabel[v] || v })])),
        (sub.files || []).length > 0 && h("ul", { class: "rows files meter" }, sub.files.map((f, k) => typeof f === "object" ? fileRow(f, k) : h("li", { class: "s", text: f }))),
        a.feedback && textBlock(typeof a.feedback === "string" ? a.feedback : JSON.stringify(a.feedback)),
        h("p", { class: "empty meter", text: "Hochladen kommt als Nächstes in nak; bis dahin in Moodle abgeben." }),
        a.url && h("a", { class: "go", href: a.url, target: "_blank", rel: "noopener", text: "In Moodle abgeben ↗" })));
  } catch (err) {
    box.replaceChildren(errorBox(err));
  }
}

const statusLabel = {
  status: "Status", gradingstatus: "Bewertung", last_modified: "zuletzt geändert", grade: "Note",
  new: "noch nichts abgegeben", draft: "Entwurf", submitted: "abgegeben", notgraded: "nicht bewertet", graded: "bewertet", reopened: "wieder geöffnet",
};

async function allAssignmentsPage(root) {
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Abgaben & Tests", h("small", { text: "Offene Aufgaben aus allen Kursen" }))));
  const grid = h("div", { class: "grid" });
  root.append(grid);
  const ta = tile("Abgaben", { cls: "w6" });
  const tq = tile("Tests", { cls: "w6", i: 1 });
  grid.append(ta, tq);
  fill(ta, () => api("moodle_assignments", { only_open: true }), assignmentRows);
  fill(tq, () => api("moodle_quizzes", { only_open: true }), quizRows);
}

async function discussionPage(root, id) {
  root.append(h("a", { class: "back", href: "#", onclick: (e) => { e.preventDefault(); history.back(); } }, svg(icons.back), "zurück"));
  const head = h("div", { class: "page-head" }, h("h1", { text: "Diskussion" }));
  root.append(head);
  const box = h("div", { class: "thread" }, skeleton());
  root.append(box);
  try {
    const r = await api("moodle_forum_posts", { discussionid: Number(id) });
    const posts = Array.isArray(r) ? r : r?.posts || [];
    if (posts[0]) head.querySelector("h1").textContent = posts[0].subject;
    box.replaceChildren(...posts.map((p, i) => h("article", { class: "post tile", vars: { "--i": Math.min(i, 10) } },
      h("header", {}, h("b", { text: p.author }), h("span", { class: "s", text: p.time })),
      i > 0 && p.subject && !String(p.subject).startsWith("Re:") && h("h3", { text: p.subject }),
      textBlock(p.message))));
  } catch (err) {
    box.replaceChildren(errorBox(err));
  }
}

// ── news & messages ─────────────────────────────────────────────────────────

async function newsPage(root) {
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Neuigkeiten", h("small", { text: "Was sich in deinen Kursen getan hat" }))));
  const grid = h("div", { class: "grid" });
  root.append(grid);
  const tNew = tile("Geändert in den letzten 14 Tagen", { cls: "w8" });
  const tNote = tile("Benachrichtigungen", { i: 1 });
  grid.append(tNew, tNote);
  fill(tNew, () => api("moodle_whats_new", { days: 14 }), (r) => {
    const list = Array.isArray(r) ? r : r?.changes || [];
    if (!list.length) return emptyRow("In den letzten 14 Tagen hat sich nichts geändert.");
    const byCourse = new Map();
    for (const x of list) {
      const k = x.course || "Kurs";
      if (!byCourse.has(k)) byCourse.set(k, { id: x.courseid, items: [] });
      byCourse.get(k).items.push(x);
    }
    return [...byCourse].map(([course, g]) => h("div", { class: "news-group" },
      h("a", { class: "news-course", href: `#/kurs/${g.id}`, text: course }),
      h("ul", { class: "rows" }, g.items.map((x, j) => h("li", {}, h("a", { class: "row", href: `#/kurs/${g.id}`, vars: { "--j": j } },
        h("span", { class: "dot", "data-src": "moodle" }),
        h("span", { class: "t" }, x.activity, h("span", { class: "s", text: (x.changes || []).map((c) => typeof c === "string" ? c : c.what || c.type || "").filter(Boolean).join(" · ") })),
        h("span", { class: "chip", text: typeName[x.type] || x.type || "" })))))));
  });
  fill(tNote, () => api("moodle_notifications", { limit: 30 }), (r) => {
    const list = Array.isArray(r) ? r : r?.notifications || [];
    if (!list.length) return emptyRow("Keine Benachrichtigungen.");
    return h("ul", { class: "rows" }, list.map((n, j) => h("li", {}, h("div", { class: `row ${n.unread ? "unread" : ""}`, vars: { "--j": j } },
      h("span", { class: "dot", "data-src": "moodle" }),
      h("span", { class: "t" }, n.subject, h("span", { class: "s", text: n.time })),
      n.unread && h("span", { class: "chip moodle", text: "neu" })))));
  });
}

async function messagesPage(root, id) {
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Nachrichten", h("small", { text: "Moodle-Nachrichten" }))));
  const layout = h("div", { class: "chat" });
  root.append(layout);
  const side = tile("Unterhaltungen", { cls: "chat-list" });
  const pane = h("section", { class: "tile chat-pane" });
  layout.append(side, pane);
  fill(side, () => api("moodle_conversations", { limit: 30 }), (r) => {
    const list = Array.isArray(r) ? r : r?.conversations || [];
    if (!list.length) return emptyRow("Keine Unterhaltungen.");
    return h("ul", { class: "rows" }, list.map((c, j) => h("li", {}, h("a", { class: "row", href: `#/nachrichten/${c.conversationid}`, vars: { "--j": j }, ...(String(c.conversationid) === String(id) ? { "aria-current": "true" } : {}) },
      h("span", { class: "dot", "data-src": "moodle" }),
      h("span", { class: "t" }, c.name || "Unterhaltung", h("span", { class: "s preview", text: c.last_message })),
      h("span", { class: "s", text: (c.last_time || "").slice(5, 10).split("-").reverse().join(".") })))));
  });
  if (!id) {
    pane.append(h("p", { class: "empty", text: "Wähle links eine Unterhaltung." }));
    return;
  }
  pane.append(skeleton());
  try {
    const r = await api("moodle_conversation_messages", { conversationid: Number(id), limit: 50 });
    const msgs = Array.isArray(r) ? r : r?.messages || [];
    const thread = h("div", { class: "msgs" }, msgs.map((m, j) => h("div", { class: `msg ${m.mine || m.from === "ich" || m.from === "me" ? "mine" : ""}`, vars: { "--j": Math.min(j, 12) } },
      h("span", { class: "s", text: [m.from, m.time].filter(Boolean).join(" · ") }),
      h("p", { text: m.text }))));
    pane.replaceChildren(thread, composer({ tool: "moodle_send_message", field: "text", base: { conversationid: Number(id) }, placeholder: "Antworten …" }));
    thread.scrollTop = thread.scrollHeight;
  } catch (err) {
    pane.replaceChildren(errorBox(err));
  }
}

// composer writes through the preview → confirm gate: the first click only
// shows what would be sent; nothing leaves nak without the second, explicit one.
function composer({ tool, field, base, placeholder }) {
  const ta = h("textarea", { rows: 2, placeholder, "aria-label": placeholder });
  const out = h("div", { class: "preview-box", hidden: true });
  const form = h("form", { class: "composer" }, ta, h("button", { class: "primary", type: "submit", text: "Vorschau" }), out);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const args = { ...base, [field]: ta.value.trim() };
    if (!args[field]) return;
    out.hidden = false;
    out.replaceChildren(h("p", { class: "empty", text: "Vorschau wird erstellt …" }));
    try {
      const pre = await post(tool, args);
      const send = h("button", { class: "primary", type: "button", text: "Verbindlich senden" });
      send.addEventListener("click", async () => {
        send.disabled = true;
        try {
          await post(tool, { ...args, confirm: true }, "", { "X-Nak-Confirm": "JA" });
          ta.value = "";
          out.replaceChildren(h("p", { class: "ok-note", text: "Gesendet." }));
          fresh = true;
          await render();
          fresh = false;
        } catch (err) {
          out.replaceChildren(errorBox(err));
        }
      });
      out.replaceChildren(h("p", { class: "empty", text: "Noch nichts gesendet. So würde es rausgehen:" }),
        h("pre", { text: typeof pre.result?.preview === "string" ? pre.result.preview : JSON.stringify(pre.result?.preview ?? pre.result, null, 2) }),
        h("div", { class: "row-actions" }, send, h("button", { class: "ghost", type: "button", text: "Abbrechen", onclick: () => { out.hidden = true; } })));
    } catch (err) {
      out.replaceChildren(errorBox(err));
    }
  });
  return form;
}

// ── modules: one unit per module, CIS and Moodle together ───────────────────

// Module numbers are upper case letters + 3 digits; matched case-sensitively
// so "TuI168_I23" (a tutorial) still yields I168 but "I23" does not count.
const nrAllRe = /[A-Z]{1,2}\d{3}/g;
const nrRe = /([A-Z]{1,2}\d{3})/;
const firstNr = (s) => (String(s || "").match(nrRe)?.[1] || "");
const nrsIn = (s) => [...new Set(String(s || "").match(nrAllRe) || [])];

const roman = { i: "1", ii: "2", iii: "3", iv: "4", v: "5", vi: "6", vii: "7", viii: "8", ix: "9", x: "10" };

// norm makes titles comparable: case, umlauts kept, "Mathematik1" → "mathematik 1",
// "II" → "2"; module numbers stay one word ("i168").
function norm(s) {
  const out = [];
  for (const raw of String(s || "").toLowerCase().split(/[^a-z0-9äöüß]+/)) {
    if (!raw) continue;
    if (/^[a-z]{1,2}\d{3}$/.test(raw)) { out.push(raw); continue; }
    for (const w of raw.replace(/([a-zäöüß])(\d)/g, "$1 $2").replace(/(\d)([a-zäöüß])/g, "$1 $2").split(" ")) out.push(roman[w] || w);
  }
  return out;
}

let unitsP = null;

function loadUnits() {
  if (unitsP && !fresh) return unitsP;
  unitsP = (async () => {
    const [g, p, c, x] = await Promise.allSettled([api("cis_grades"), api("cis_progress"), api("moodle_courses", { classification: "all" }), api("cis_list_klausuren")]);
    const map = new Map();
    const add = (nr, title) => {
      if (!nr) return null;
      let u = map.get(nr);
      if (!u) map.set(nr, (u = { nr, title: "", grade: null, plan: null, courses: [], exams: [], aliases: [] }));
      u.title ||= title;
      return u;
    };
    for (const sem of p.value?.semesters || []) for (const m of sem.modules || []) { const u = add(firstNr(m.module_nr), m.title); if (u) u.plan = m; }
    for (const m of g.value?.overview?.modules || []) { const u = add(firstNr(m.module_nr), m.title); if (u) u.grade = m; }
    const units = [...map.values()];
    const titled = units.map((u) => ({ u, t: ` ${norm(u.title).join(" ")} ` })).filter((x) => x.t.trim()).sort((a, b) => b.t.length - a.t.length);
    const byTitle = (s) => { const n = ` ${norm(s).join(" ")} `; return titled.find((x) => n.includes(x.t))?.u; };
    // Exams can run under other numbers than the grade (new exam regulations:
    // "A222,I222" for the module graded as I168) — match by number, then by
    // title, and keep the extra numbers as aliases of the same unit.
    const alias = new Map();
    for (const e of x.value || []) {
      const nrs = nrsIn(e.module_nr);
      const u = nrs.map((nr) => map.get(nr)).find(Boolean) || byTitle(e.title);
      if (!u) continue;
      u.exams.push(e);
      for (const nr of nrs) if (!map.has(nr)) { alias.set(nr, u); if (!u.aliases.includes(nr)) u.aliases.push(nr); }
    }
    const orphans = [];
    for (const course of c.value || []) {
      const label = `${course.name} ${course.shortname}`;
      let u = nrsIn(label).map((nr) => map.get(nr)).find(Boolean);
      u ||= nrsIn(label).map((nr) => alias.get(nr)).find(Boolean) || byTitle(courseTitle(course));
      if (u) u.courses.push(course);
      else orphans.push(course);
    }
    const rank = (c) => (c.state === "current" ? 0 : c.state === "future" ? 1 : 2) + (/tutori/i.test(c.name) ? 0.5 : 0);
    for (const u of units) u.courses.sort((a, b) => rank(a) - rank(b));
    const byNr = { get: (nr) => map.get(nr) || alias.get(nr) };
    return { units, orphans, byNr, unitOf: (s) => nrsIn(s).map((nr) => byNr.get(nr)).find(Boolean) || byTitle(s), unitOfCourse: (id) => units.find((u) => u.courses.some((x) => String(x.id) === String(id))) };
  })();
  return unitsP;
}

function matches(q, words) {
  const set = words;
  return norm(q).every((t) => /^\d+$/.test(t) ? set.includes(t) : set.some((w) => w.startsWith(t)));
}

function unitLabel(u) {
  if (u.grade?.grade) return `Note ${u.grade.grade}`;
  if (u.courses.some((c) => c.state === "current")) return "aktuell";
  return u.nr;
}

async function unitsPage(root) {
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Module", h("small", { text: "CIS und Moodle zusammen" }))));
  const box = h("div", { class: "mods" }, [0, 1, 2].map((i) => tile("", { i }, skeleton())));
  root.append(box);
  try {
    const { units } = await loadUnits();
    const list = [...units].sort((a, b) => (Number(b.courses.some((c) => c.state === "current")) - Number(a.courses.some((c) => c.state === "current"))) || a.nr.localeCompare(b.nr));
    box.replaceChildren(...list.map((u, i) => unitCard(u, i)));
  } catch (err) {
    box.replaceChildren(errorBox(err));
  }
}

function unitCard(u, i) {
  const t = tile("", { i: Math.min(i, 12) },
    h("a", { class: "mod", href: `#/modul/${u.nr}` },
      h("span", { class: "nr", text: u.nr }),
      h("span", { class: "name", text: u.title || u.nr }),
      h("span", { class: "tags" },
        u.courses.some((c) => c.state === "current") && h("span", { class: "chip moodle", text: "läuft" }),
        u.grade?.grade ? h("span", { class: `chip ${gradeClass(u.grade)}`, text: u.grade.grade }) : u.grade?.status && h("span", { class: `chip ${gradeClass(u.grade)}`, text: u.grade.status }),
        u.courses.length > 0 && h("span", { class: "chip", text: `${u.courses.length} Moodle-Kurs${u.courses.length > 1 ? "e" : ""}` }))));
  t.querySelector(".tile-head").remove();
  return t;
}

// The unified module page: CIS facts and every linked Moodle course.
async function unitPage(root, nr, tab = "", courseArg = "") {
  root.append(h("a", { class: "back", href: "#", onclick: (e) => { e.preventDefault(); history.back(); } }, svg(icons.back), "zurück"));
  const title = h("h1", { text: nr });
  const head = h("div", { class: "page-head" }, title);
  root.append(head);
  const tabsBox = h("div");
  const body = h("div", { class: "course-body" }, skeleton());
  root.append(tabsBox, body);
  let u;
  try {
    u = (await loadUnits()).byNr.get(nr);
  } catch (err) {
    body.replaceChildren(errorBox(err));
    return;
  }
  const courses = u?.courses || [];
  const cid = courseArg ? Number(courseArg) : courses[0]?.id;
  if (u && u.nr !== nr) { location.replace(`#/modul/${u.nr}${tab ? "/" + tab : ""}${courseArg ? "/" + courseArg : ""}`); return; }
  title.replaceChildren(u?.title || nr, h("small", { text: [[nr, ...(u?.aliases || [])].join(", "), u?.plan?.credits && `${u.plan.credits} Credits`, u?.grade?.status].filter(Boolean).join(" · ") }));
  const base = `#/modul/${nr}`;
  const link = (t, c = cid) => `${base}${t ? "/" + t : ""}${c && c !== courses[0]?.id ? "/" + c : t && c ? "/" + c : ""}`;
  tabsBox.append(h("nav", { class: "tabs", "aria-label": "Bereiche" },
    [["", "Überblick"], ...(courses.length ? [["inhalt", "Inhalt"], ["forum", "Forum"], ["abgaben", "Abgaben & Tests"]] : [])].map(([k, label]) =>
      h("a", { href: link(k), text: label, ...(k === tab ? { "aria-current": "page" } : {}) }))));
  if (tab && courses.length > 1) {
    tabsBox.append(h("div", { class: "course-switch", role: "group", "aria-label": "Moodle-Kurs" }, courses.map((c) =>
      h("a", { href: link(tab, c.id), class: String(c.id) === String(cid) ? "on" : "", text: `${courseTitle(c)}${c.state === "current" ? "" : ` · ${c.state === "past" ? "vergangen" : c.state}`}` }))));
  }
  if (tab) return courseTab(body, cid, tab);

  // overview
  const grid = h("div", { class: "grid" });
  body.replaceChildren(grid);
  const tGrade = tile("Note & Prüfung", { cls: "half", i: 0 });
  const tPlan = tile("Studienplan & Klausuren", { cls: "half", i: 1 });
  const tNext = tile("Nächste Termine", { cls: "half", i: 2 });
  const tMoodle = tile("Moodle", { cls: "w6", i: 3 });
  const tNews = tile("Neu in Moodle", { cls: "w6", i: 4 });
  const tEv = tile("Altklausuren · EduVault", { cls: "w12", i: 5 });
  grid.append(tGrade, tPlan, tNext, tMoodle, tNews, tEv);
  fill(tEv, () => api("eduvault_module_exams", { title: u?.title || nr }), evRows).then(() => {
    const err = tEv.querySelector(".err");
    if (err && /eingerichtet|Einstellungen/.test(err.textContent)) err.replaceWith(h("p", { class: "empty" }, "Hinterlege in den ", h("a", { href: "#/einstellungen", text: "Einstellungen" }), " deinen EduVault-Zugang, dann stehen hier die Altklausuren zu diesem Modul."));
  });
  const data = api("nak_module", { module_nr: nr });
  fill(tGrade, () => data, (m) => gradeTile(m, u));
  const examsOf = (m) => dedupeExams([...(m.exams || []), ...(u?.exams || [])]);
  fill(tPlan, () => data, (m) => planTile({ ...m, exams: examsOf(m) }));
  fill(tNext, () => data, (m) => {
    const now = new Date();
    const exams = examsOf(m).filter((e) => e.registered && parseDE(e.start) >= startOfDay(now))
      .map((e) => ({ date: fmtShort.format(parseDE(e.start)), start: e.start.slice(11, 16), end: (e.ende || "").slice(11, 16), kind: "Klausur", room: "", lecturer: "", at: parseDE(e.start) }));
    const evs = [...exams, ...(m.next_sessions || []).map((e) => ({ ...e, at: parseDE(`${e.date} ${e.start}`) }))].sort((a, b) => (a.at || 0) - (b.at || 0));
    if (!evs.length) return emptyRow("Keine Termine in den nächsten 60 Tagen.");
    return h("ul", { class: "rows" }, evs.map((e, j) => h("li", {}, h("div", { class: "row", vars: { "--j": j } },
      h("span", { class: "dot", "data-src": "cis" }),
      h("span", { class: "t" }, `${e.date} · ${e.start}–${e.end}`, h("span", { class: "s", text: [e.room, e.lecturer].filter(Boolean).join(" · ") })),
      h("span", { class: "chip cis", text: e.kind || "CIS" })))));
  });
  fill(tMoodle, () => Promise.all([data, courses.length ? api("moodle_assignments", { only_open: true }) : []]), ([m, open]) => {
    if (!courses.length) return emptyRow("Kein Moodle-Kurs zu diesem Modul.");
    const ids = new Set(courses.map((c) => c.id));
    const mine = (open || []).filter((a) => ids.has(a.courseid));
    return [h("ul", { class: "rows" }, courses.map((c, j) => h("li", {}, h("a", { class: "row", href: link("inhalt", c.id), vars: { "--j": j } },
      h("span", { class: "dot", "data-src": "moodle" }),
      h("span", { class: "t" }, courseTitle(c), h("span", { class: "s", text: [c.state === "current" ? "aktuell" : c.state === "past" ? "vergangen" : c.state, c.progress && `Fortschritt ${c.progress}`].filter(Boolean).join(" · ") })),
      h("span", { class: "chip moodle", text: "Inhalt" }))))),
    mine.length > 0 && h("div", { class: "after" }, h("h2", { text: "Offene Abgaben" }), assignmentRows(mine))];
  });
  fill(tNews, () => courses.length ? api("moodle_whats_new", { days: 30, courseids: courses.map((c) => c.id) }) : Promise.resolve([]), (r) => {
    const list = Array.isArray(r) ? r : r?.changes || [];
    if (!list.length) return emptyRow(courses.length ? "In den letzten 30 Tagen nichts Neues." : "Kein Moodle-Kurs zu diesem Modul.");
    return h("ul", { class: "rows" }, list.slice(0, 8).map((x, j) => h("li", {}, h("a", { class: "row", href: link("inhalt", x.courseid), vars: { "--j": j } },
      h("span", { class: "dot", "data-src": "moodle" }),
      h("span", { class: "t" }, x.activity, h("span", { class: "s", text: [x.course, ...(x.changes || []).map((c) => typeof c === "string" ? c : c.what || c.type || "")].filter(Boolean).join(" · ") })),
      h("span", { class: "chip", text: typeName[x.type] || x.type || "" })))));
  });
}

function gradeTile(m, u) {
  const g = m.grade || u?.grade;
  const d = m.distribution;
  if (!g) return emptyRow("Noch keine Leistung eingetragen.");
  const out = [h("div", { class: "avg" }, g.grade_value ? decimal(g.grade_value, 1) : h("span", { class: "big", text: "–" }), h("span", { class: `chip ${gradeClass(g)}`, text: g.status || "" }))];
  out.push(h("dl", { class: "kv meter" },
    h("dt", { text: "Prüfung" }), h("dd", { text: g.exam_date || "–" }),
    h("dt", { text: "Versuch" }), h("dd", { text: g.attempt || "–" }),
    d?.count > 0 && [h("dt", { text: "Jahrgang" }), h("dd", { text: `Schnitt ${String(d.average).replace(".", ",")} · ${d.count} Ergebnisse${d.percentile ? ` · besser als ${Math.round(d.percentile)} %` : ""}` })]));
  if (d?.buckets?.length) {
    const max = Math.max(...d.buckets.map((b) => b.count), 1);
    out.push(h("div", { class: "dist", role: "img", "aria-label": "Notenverteilung" }, d.buckets.map((b, j) =>
      h("span", { class: `col ${b.grade === g.grade ? "mine" : ""}`, title: `${b.grade}: ${b.count}`, vars: { "--h": b.count / max, "--j": j } }, h("i"), h("small", { text: b.grade })))));
  }
  return out;
}

// One exam often exists once per group (Zenturie) with its own id; show it
// once, preferring the entry you are registered for.
function dedupeExams(list) {
  const out = new Map();
  for (const e of list) {
    const k = `${e.start}|${norm(e.title).join(" ")}`;
    const prev = out.get(k);
    if (!prev || (e.registered && !prev.registered)) out.set(k, e);
  }
  return [...out.values()].sort((a, b) => (parseDE(a.start) || 0) - (parseDE(b.start) || 0));
}

function planTile(m) {
  const sp = m.studienplan;
  const ex = m.exams || [];
  if (!sp && !ex.length) return emptyRow("Nicht im Studienplan.");
  return [sp && h("dl", { class: "kv" },
    h("dt", { text: "Credits" }), h("dd", { text: sp.credits }),
    h("dt", { text: "Prüfungsform" }), h("dd", { text: sp.exam_form_name || sp.exam_form || "–" }),
    sp.exam_semester && [h("dt", { text: "Prüfungssemester" }), h("dd", { text: sp.exam_semester })]),
    ex.length > 0 && h("ul", { class: "rows meter" }, ex.map((e, j) => h("li", {}, h("div", { class: "row", vars: { "--j": j } },
      h("span", { class: "dot", "data-src": "cis" }),
      h("span", { class: "t" }, e.title || "Prüfung", h("span", { class: "s", text: [e.start, e.deadlines?.register_closes && `Anmeldung bis ${e.deadlines.register_closes}`].filter(Boolean).join(" · ") })),
      h("span", { class: `chip ${e.registered ? "ok" : ""}`, text: e.registered ? "angemeldet" : (e.action_label || "offen") })))))];
}

// #/kurs/ID: a course that belongs to a module opens inside the module page.
async function coursePage(root, id, tab = "inhalt") {
  try {
    const u = (await loadUnits()).unitOfCourse(id);
    if (u) { location.replace(`#/modul/${u.nr}/${tab || "inhalt"}/${id}`); return; }
  } catch {}
  root.append(h("a", { class: "back", href: "#/kurse" }, svg(icons.back), "Kurse"));
  const title = h("h1", { text: "Kurs" });
  const tabs = h("nav", { class: "tabs", "aria-label": "Kursbereiche" },
    [["inhalt", "Inhalt"], ["forum", "Forum"], ["abgaben", "Abgaben & Tests"], ["info", "Info"]].map(([k, label]) =>
      h("a", { href: `#/kurs/${id}${k === "inhalt" ? "" : "/" + k}`, text: label, ...(k === tab ? { "aria-current": "page" } : {}) })));
  root.append(h("div", { class: "page-head" }, title), tabs);
  const body = h("div", { class: "course-body" }, skeleton());
  root.append(body);
  api("moodle_courses", { classification: "all" }).then((cs) => {
    const c = cs.find((x) => String(x.id) === String(id));
    if (c) title.replaceChildren(courseTitle(c), h("small", { text: [c.shortname, c.progress && `Fortschritt ${c.progress}`].filter(Boolean).join(" · ") }));
  }).catch(() => {});
  return courseTab(body, Number(id), tab);
}

async function courseTab(body, cid, tab) {
  try {
    if (tab === "inhalt") {
      const [secs, assigns] = await Promise.all([api("moodle_course_contents", { courseid: cid }), api("moodle_assignments", { courseid: cid, only_open: false }).catch(() => [])]);
      const byCm = Object.fromEntries((assigns || []).map((a) => [a.cmid, a.assignid]));
      const visible = (secs || []).filter((s) => (s.modules || []).length || s.summary);
      if (!visible.length) return body.replaceChildren(emptyRow("Dieser Kurs hat keine sichtbaren Inhalte."));
      body.replaceChildren(...visible.map((s, i) => h("details", { class: "section tile", open: i < 3 || visible.length <= 4, vars: { "--i": Math.min(i, 8) } },
        h("summary", {}, h("span", { class: "t", text: s.name || `Abschnitt ${s.section}` }), h("span", { class: "s", text: `${(s.modules || []).length} Einträge` })),
        textBlock(s.summary),
        h("ul", { class: "items" }, (s.modules || []).map((m, j) => moduleItem(m, cid, byCm, j))))));
    } else if (tab === "forum") {
      const ds = await api("moodle_forum_discussions", { courseid: cid, limit: 15 });
      const list = Array.isArray(ds) ? ds : ds?.discussions || [];
      if (!list.length) return body.replaceChildren(emptyRow("Keine Forendiskussionen."));
      body.replaceChildren(tile("Diskussionen", { cls: "w12" }, h("ul", { class: "rows" }, list.map((d, j) => h("li", {}, h("a", { class: "row", href: `#/diskussion/${d.discussionid}`, vars: { "--j": j } },
        h("span", { class: "dot", "data-src": "moodle" }),
        h("span", { class: "t" }, d.subject, h("span", { class: "s", text: [d.forum, d.author, d.last_activity].filter(Boolean).join(" · ") }), d.preview && h("span", { class: "s preview", text: d.preview })),
        h("span", { class: "chip", text: `${d.replies || 0} Antw.` })))))));
    } else if (tab === "abgaben") {
      const [as, qs] = await Promise.all([api("moodle_assignments", { courseid: cid, only_open: false }), api("moodle_quizzes", { courseid: cid, only_open: false }).catch(() => [])]);
      body.replaceChildren(h("div", { class: "grid" },
        tile("Abgaben", { cls: "w6" }, assignmentRows(as || [])),
        tile("Tests", { cls: "w6", i: 1 }, quizRows(qs || []))));
    } else {
      const info = await api("moodle_course_info", { courseid: cid });
      body.replaceChildren(tile("Kurs", { cls: "w12" },
        h("dl", { class: "kv" }, Object.entries(info).filter(([k, v]) => typeof v !== "object" && !["url", "summary", "id"].includes(k) && v !== "").map(([k, v]) => [h("dt", { text: k }), h("dd", { text: v })])),
        textBlock(info.summary)));
    }
  } catch (err) {
    body.replaceChildren(errorBox(err));
  }
}

// ── settings ────────────────────────────────────────────────────────────────

async function settingsPage(root) {
  root.append(h("div", { class: "page-head" }, h("h1", {}, "Einstellungen", h("small", { text: "Gilt für diese naknak-Instanz" }))));
  const grid = h("div", { class: "grid" });
  root.append(grid);
  let st;
  try {
    st = await (await fetch("/api/settings", { credentials: "same-origin" })).json();
  } catch (err) {
    grid.append(errorBox(err));
    return;
  }

  // EduVault
  const ev = st.eduvault || {};
  const tEv = tile("EduVault · Altklausuren", { cls: "w8", i: 0 });
  const status = h("p", { class: "ev-status" }, ev.configured
    ? [h("span", { class: "chip ok", text: "verbunden" }), ` ${ev.token_hint || ""} · ${ev.url}`, ev.source === "env" ? " (aus Umgebungsvariablen)" : ""]
    : [h("span", { class: "chip", text: "nicht eingerichtet" }), " Mit Zugangsdaten zeigt naknak auf jeder Modulseite die passenden Altklausuren."]);
  const url = h("input", { name: "url", type: "url", value: ev.url || "https://eduvault4.de", autocomplete: "off", spellcheck: "false" });
  const token = h("input", { name: "token", autocomplete: "off", spellcheck: "false", placeholder: "evm_…" });
  const secret = h("input", { name: "secret", type: "password", autocomplete: "off", spellcheck: "false", placeholder: "Base32, z.B. JBSW…" });
  const msg = h("div", { class: "form-msg", "aria-live": "polite" });
  const save = h("button", { class: "primary", type: "submit" }, h("span", { class: "label", text: "Prüfen & speichern" }), h("span", { class: "spin", "aria-hidden": "true" }));
  const form = h("form", { class: "settings-form" },
    h("label", {}, "Adresse", url),
    h("label", {}, "Token", token),
    h("label", {}, "TOTP-Secret", secret),
    h("p", { class: "empty", text: "Beides bekommst du in EduVault unter Profil → MCP / KI-Zugang → Zugangsdaten erstellen. naknak prüft die Daten mit einer Anmeldung, bevor es sie speichert; sie verlassen diesen Server danach nur Richtung EduVault." }),
    h("div", { class: "row-actions" }, save,
      ev.configured && ev.source === "settings" && h("button", { class: "ghost", type: "button", text: "Entfernen", onclick: async () => {
        await fetch("/api/settings/eduvault", { method: "DELETE", credentials: "same-origin" });
        unitsP = null;
        render();
      } })),
    msg);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    save.classList.add("busy");
    save.disabled = true;
    msg.replaceChildren();
    try {
      const res = await fetch("/api/settings/eduvault", {
        method: "PUT", credentials: "same-origin", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url: url.value, token: token.value, secret: secret.value }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
      msg.replaceChildren(h("p", { class: "ok-note", text: "Verbunden. Altklausuren erscheinen jetzt auf den Modulseiten." }));
      token.value = secret.value = "";
      setTimeout(render, 900);
    } catch (err) {
      msg.replaceChildren(errorBox(err));
    } finally {
      save.classList.remove("busy");
      save.disabled = false;
    }
  });
  tEv.append(status, form);

  // account & instance
  const acc = st.account || {};
  const tAcc = tile("Konto & Instanz", { i: 1 },
    h("dl", { class: "kv" },
      h("dt", { text: "NAK-Konto" }), h("dd", { text: acc.user || "–" }),
      h("dt", { text: "Herkunft" }), h("dd", { text: acc.source === "env" ? "Umgebungsvariablen" : acc.source === "file" ? "Web-Anmeldung" : "–" }),
      h("dt", { text: "Modus" }), h("dd", {}, st.read_only ? h("span", { class: "chip due", text: "nur lesen" }) : h("span", { class: "chip ok", text: "voll" })),
      h("dt", { text: "Version" }), h("dd", { text: st.version || "–" }),
      h("dt", { text: "Daten" }), h("dd", { class: "mono-ish", text: st.data_dir || "–" })));

  // appearance
  const tLook = tile("Darstellung", { i: 2 });
  const cur = localStorage.getItem("nak-theme") || "system";
  const seg = h("div", { class: "filters", role: "group", "aria-label": "Design" }, themes.map((t) =>
    h("button", { type: "button", "aria-pressed": t === cur ? "true" : "false", text: { system: "System", light: "Hell", dark: "Dunkel" }[t], onclick: (e) => {
      localStorage.setItem("nak-theme", t);
      for (const b of seg.children) b.setAttribute("aria-pressed", b === e.currentTarget ? "true" : "false");
      applyTheme(t);
    } })));
  tLook.append(seg, h("p", { class: "empty meter", text: "Bewegungen richten sich nach „Bewegung reduzieren“ im Betriebssystem." }));

  // cache
  const tCache = tile("Zwischenspeicher", { cls: "w8", i: 3 },
    h("p", { class: "empty", text: "naknak merkt sich Antworten von CIS und Moodle und zeigt sie sofort an; veraltete Daten werden im Hintergrund erneuert (Nachrichten nach 1 Minute, Fristen nach 5, Stundenplan nach 30, Noten nach 2 Stunden)." }),
    h("div", { class: "row-actions" }, h("button", { class: "ghost", type: "button", text: "Zwischenspeicher leeren", onclick: async (e) => {
      e.currentTarget.disabled = true;
      await fetch("/api/cache/clear", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: "{}" });
      unitsP = null;
      alertBox("Zwischenspeicher geleert.");
      e.currentTarget.disabled = false;
    } })));
  grid.append(tEv, tAcc, tLook, tCache);
}

// ── EduVault on module pages ────────────────────────────────────────────────

const evType = { exam: "Klausur", probeklausur: "Probeklausur", note: "Lernzettel", script: "Skript" };

function evRows(r) {
  const docs = r.documents || [];
  if (!docs.length) return emptyRow(`Noch keine Altklausuren zu „${r.module}“ in EduVault.`);
  let filter = "all", all = false;
  const counts = { exam: 0, probeklausur: 0, solved: 0 };
  for (const d of docs) { if (d.type in counts) counts[d.type]++; if (d.has_solutions) counts.solved++; }
  const list = h("ul", { class: "rows" });
  const more = h("button", { class: "ghost more-btn", type: "button" });
  const draw = () => {
    const shown = docs.filter((d) => filter === "all" || (filter === "solved" ? d.has_solutions : d.type === filter));
    list.replaceChildren(...(all ? shown : shown.slice(0, 8)).map((d, j) => h("li", {}, h("button", { class: "row file", type: "button", vars: { "--j": Math.min(j, 12) }, onclick: (e) => openEduVault(d.id, e.currentTarget) },
      h("span", { class: "ext ev", text: /^\d{4}$/.test(String(d.jahr)) ? String(d.jahr).slice(-2) : "EV" }),
      h("span", { class: "t" }, `${evType[d.type] || "Dokument"} ${d.jahr || ""}`.trim(), h("span", { class: "s", text: [d.modul, Array.isArray(d.dozenten) ? d.dozenten.join(", ") : d.dozenten, Number(d.rating) > 0 && `★ ${Number(d.rating).toFixed(1)}`, d.download_count > 0 && `${d.download_count}× geladen`].filter(Boolean).join(" · ") })),
      h("span", { class: `chip ${d.has_solutions ? "ok" : ""}`, text: d.has_solutions ? "mit Lösung" : "öffnen" })))));
    if (!shown.length) list.append(h("li", { class: "empty", text: "Keine Dokumente in dieser Auswahl." }));
    more.hidden = shown.length <= 8;
    more.textContent = all ? "Weniger anzeigen" : `Alle ${shown.length} anzeigen`;
  };
  more.addEventListener("click", () => { all = !all; draw(); });
  const opts = [["all", `Alle ${docs.length}`], ["exam", `Klausuren ${counts.exam}`], ["probeklausur", `Probeklausuren ${counts.probeklausur}`], ["solved", `mit Lösung ${counts.solved}`]].filter(([k], i) => i === 0 || (k === "solved" ? counts.solved : counts[k]));
  const filters = h("div", { class: "filters ev-filters", role: "group", "aria-label": "Auswahl" }, opts.map(([k, label]) =>
    h("button", { type: "button", "aria-pressed": k === filter ? "true" : "false", text: label, onclick: (e) => {
      filter = k;
      all = false;
      for (const b of filters.children) b.setAttribute("aria-pressed", b === e.currentTarget ? "true" : "false");
      draw();
    } })));
  draw();
  return [filters, list, more];
}

async function openEduVault(id, btn) {
  const win = window.open("", "_blank");
  btn?.classList.add("busy");
  try {
    const r = await post("eduvault_download", { exam_id: id });
    if (!r.download_url) throw new Error("Datei konnte nicht bereitgestellt werden");
    if (win) win.location = `${r.download_url}?inline=1`;
  } catch (err) {
    win?.close();
    alertBox(err.message);
  } finally {
    btn?.classList.remove("busy");
  }
}

// ── header, theme, routing ──────────────────────────────────────────────────

const routes = [
  [/^#?\/?$/, overview, "#/"],
  [/^#\/woche$/, week, "#/woche"],
  [/^#\/kurse$/, coursesPage, "#/kurse"],
  [/^#\/kurs\/(\d+)(?:\/(forum|abgaben|info))?$/, coursePage, "#/kurse"],
  [/^#\/abgaben$/, allAssignmentsPage, "#/kurse"],
  [/^#\/abgabe\/(\d+)$/, assignmentPage, "#/kurse"],
  [/^#\/diskussion\/(\d+)$/, discussionPage, "#/kurse"],
  [/^#\/neu$/, newsPage, "#/neu"],
  [/^#\/nachrichten(?:\/(\d+))?$/, messagesPage, "#/nachrichten"],
  [/^#\/noten$/, gradesPage, "#/noten"],
  [/^#\/einstellungen$/, settingsPage, "#/einstellungen"],
  [/^#\/module$/, unitsPage, "#/kurse"],
  [/^#\/modul\/([A-Z]{1,2}\d{3})(?:\/(inhalt|forum|abgaben))?(?:\/(\d+))?$/, unitPage, "#/kurse"],
  [/^#\/module\/([A-Za-z]{1,2}\d{3})$/, (root, nr) => { location.replace(`#/modul/${nr.toUpperCase()}`); }, "#/kurse"],
];

function stamp() {
  const el = $(".stamp");
  if (!el || !newest) return;
  const mins = Math.round((Date.now() - newest) / 6e4);
  el.textContent = mins < 1 ? "Stand: gerade eben" : `Stand: vor ${mins} min`;
}
setInterval(stamp, 30_000);

const themes = ["system", "light", "dark"];
const themeLabel = { system: "Design: wie System", light: "Design: hell", dark: "Design: dunkel" };

function applyTheme(t) {
  if (t === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = t;
  const btn = $(".theme-btn");
  if (btn) {
    btn.replaceChildren(svg(icons[t]), h("span", { class: "label", text: themeLabel[t] }));
    btn.title = themeLabel[t];
  }
}

function switchTheme(e) {
  const cur = localStorage.getItem("nak-theme") || "system";
  const next = themes[(themes.indexOf(cur) + 1) % themes.length];
  localStorage.setItem("nak-theme", next);
  if (!document.startViewTransition || reduced.matches) return applyTheme(next);
  const r = e.currentTarget.getBoundingClientRect();
  const x = r.left + r.width / 2, y = r.top + r.height / 2;
  const root = document.documentElement;
  root.style.setProperty("--tx", `${x}px`);
  root.style.setProperty("--ty", `${y}px`);
  root.style.setProperty("--tr", `${Math.hypot(Math.max(x, innerWidth - x), Math.max(y, innerHeight - y))}px`);
  root.classList.add("theme-switch");
  document.startViewTransition(() => applyTheme(next)).finished.finally(() => root.classList.remove("theme-switch"));
}

function header() {
  const links = [["Übersicht", "#/"], ["Woche", "#/woche"], ["Kurse", "#/kurse"], ["Neuigkeiten", "#/neu"], ["Nachrichten", "#/nachrichten"], ["Noten", "#/noten"]];
  const refresh = h("button", { class: "icon-btn", type: "button", title: "Neu laden (frisch aus CIS und Moodle)", "aria-label": "Neu laden" }, svg(icons.refresh));
  refresh.addEventListener("click", async () => {
    refresh.classList.add("spin");
    fresh = true;
    try { await render(); } finally { fresh = false; refresh.classList.remove("spin"); }
  });
  return h("header", { class: "top" }, h("div", { class: "top-in" },
    h("a", { class: "mark", href: "#/", "aria-label": "naknak – Übersicht" }, h("img", { src: "/assets/naknak.svg", alt: "", width: 26, height: 26 }), "naknak"),
    h("nav", { "aria-label": "Bereiche" }, links.map(([t, href]) => h("a", { href, text: t }))),
    h("div", { class: "tools" },
      h("span", { class: "stamp", "aria-live": "polite" }),
      refresh,
      h("a", { class: "icon-btn", href: "#/einstellungen", title: "Einstellungen", "aria-label": "Einstellungen" }, svg(icons.gear)),
      h("button", { class: "icon-btn theme-btn", type: "button", onclick: switchTheme }),
      h("form", { method: "post", action: "/logout" }, h("button", { class: "icon-btn", type: "submit", title: "Abmelden", "aria-label": "Abmelden" }, svg(["M15 4h4v16h-4", "M10 8l-4 4 4 4", "M6 12h10"]))))));
}

let renderSeq = 0;

function route() {
  const hash = location.hash || "#/";
  const match = routes.find(([re]) => re.test(hash));
  if (!match) return null;
  const [re, page, nav] = match;
  const [, ...args] = hash.match(re);
  return { page, nav, args: args.map((a) => a && (/^[a-z]{1,2}\d{3}$/i.test(a) ? a.toUpperCase() : a)) };
}

async function render() {
  const seq = ++renderSeq;
  const r = route();
  if (!r) { location.hash = "#/"; return; }
  for (const a of document.querySelectorAll(".top nav a")) {
    if (a.getAttribute("href") === r.nav) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  newest = null;
  staleSeen.clear();
  const main = h("main", { id: "main" });
  const swap = () => { $("#main").replaceWith(main); scrollTo(0, 0); };
  if (document.startViewTransition && !reduced.matches && seq > 1) await document.startViewTransition(swap).updateCallbackDone;
  else swap();
  await r.page(main, ...r.args);
  await settle();
  if (seq === renderSeq && staleSeen.size) revalidate(seq, r);
}

// revalidate asks the server for fresh versions of everything that came from
// stale cache and redraws the page in place — without animation — if any of
// it changed.
async function revalidate(seq, r) {
  const calls = [...staleSeen.values()];
  staleSeen.clear();
  const el = $(".stamp");
  if (el) el.textContent = "aktualisiere …";
  const results = await Promise.allSettled(calls.map((c) => api(c.tool, c.args, { wait: true })));
  const changed = results.some((x, i) => x.status === "fulfilled" && JSON.stringify(x.value) !== calls[i].text);
  if (seq !== renderSeq) return;
  if (!changed) { newest = new Date(); stamp(); return; }
  const next = h("main", { id: "main", class: "quiet" });
  const keepY = scrollY;
  newest = null;
  await r.page(next, ...r.args);
  await settle();
  if (seq !== renderSeq) return;
  $("#main").replaceWith(next);
  scrollTo(0, keepY);
}

function boot() {
  const app = $("#app");
  app.replaceWith(header(), h("main", { id: "main" }));
  applyTheme(localStorage.getItem("nak-theme") || "system");
  addEventListener("hashchange", render);
  render();
}

boot();
