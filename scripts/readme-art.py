#!/usr/bin/env python3
"""Draws the README artwork into docs/readme/.

    python3 -m venv /tmp/artvenv && /tmp/artvenv/bin/pip install uharfbuzz fonttools
    /tmp/artvenv/bin/python scripts/readme-art.py

Text is set with HarfBuzz and stored as outlines, so the SVGs look identical on
GitHub (SVGs are shown as images there: no web fonts). Fonts (OFL) are cached in
~/.cache/nak-readme-fonts. All data shown is made up.
"""

import math
import random
import urllib.request
from pathlib import Path

import uharfbuzz as hb
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "docs" / "readme"
CACHE = Path.home() / ".cache" / "nak-readme-fonts"
FONTS = {
    "grotesk": "spacegrotesk/SpaceGrotesk%5Bwght%5D.ttf",
    "mono": "ibmplexmono/IBMPlexMono-Medium.ttf",
}


def font_file(key, wght=None):
    CACHE.mkdir(parents=True, exist_ok=True)
    src = CACHE / FONTS[key].split("/")[-1].replace("%5B", "[").replace("%5D", "]")
    if not src.exists():
        urllib.request.urlretrieve("https://cdn.jsdelivr.net/gh/google/fonts@main/ofl/" + FONTS[key], src)
    if wght is None:
        return src
    out = CACHE / f"{key}-{wght}.ttf"
    if not out.exists():
        f = TTFont(src)
        instancer.instantiateVariableFont(f, {"wght": wght}, inplace=True)
        f.save(out)
    return out


class Face:
    def __init__(self, path):
        self.tt = TTFont(path)
        self.glyphs = self.tt.getGlyphSet()
        self.order = self.tt.getGlyphOrder()
        self.upem = self.tt["head"].unitsPerEm
        self.hb = hb.Font(hb.Face(hb.Blob.from_file_path(str(path))))

    def path(self, text, size, x, y, tracking=0, anchor="start"):
        buf = hb.Buffer()
        buf.add_str(text)
        buf.guess_segment_properties()
        hb.shape(self.hb, buf, {"kern": True, "liga": True})
        tr = tracking * self.upem / size if size else 0
        shaped, adv = [], 0
        for info, pos in zip(buf.glyph_infos, buf.glyph_positions):
            shaped.append((self.order[info.codepoint], adv + pos.x_offset, pos.y_offset))
            adv += pos.x_advance + tr
        width = (adv - tr) * size / self.upem
        if anchor == "middle":
            x -= width / 2
        elif anchor == "end":
            x -= width
        s = size / self.upem
        pen = SVGPathPen(self.glyphs)
        for name, gx, gy in shaped:
            self.glyphs[name].draw(TransformPen(pen, (s, 0, 0, -s, x + gx * s, y - gy * s)))
        return pen.getCommands(), width


def palette(dark):
    return {
        "bg0": "#07101f" if dark else "#f6f4ee",
        "bg1": "#0c1d3a" if dark else "#e6ecf5",
        "ink": "#f2f5fb" if dark else "#0d1b33",
        "text": "#b8c4da" if dark else "#36445f",
        "dim": "#6c7a96" if dark else "#7b8499",
        "grid": "#7fa7e6" if dark else "#2f5fa8",
        "cis": "#6fb2ff" if dark else "#1f63c4",
        "moodle": "#f39a4c" if dark else "#c8601a",
        "due": "#ffc857" if dark else "#d08a00",
        "ok": "#7ed4a6" if dark else "#1f8a55",
        "card": "#0a1528" if dark else "#ffffff",
    }


def hero(dark):
    W, H = 1280, 440
    p = palette(dark)
    g_bold = Face(font_file("grotesk", 700))
    g_med = Face(font_file("grotesk", 500))
    mono = Face(font_file("mono"))

    word, ww = g_bold.path("nak", 150, 96, 214, tracking=-6)
    api, _ = g_med.path("api", 150, 96 + ww + 14, 214, tracking=-6)
    tag1, _ = g_med.path("CIS and Moodle of the NORDAKADEMIE, as one MCP server.", 24, 100, 268)
    tag2, _ = g_med.path("Grades, exams, timetable and every deadline in one place.", 24, 100, 300)
    tag3, _ = g_med.path("Nothing binding happens without your yes.", 24, 100, 332)

    chips, cx = [], 100
    for label in ["GO", "MCP · STDIO", "78 TOOLS", "CIS", "MOODLE", "READ-ONLY SWITCH"]:
        path, w = mono.path(label, 12.5, cx + 14, 384 + 4.4, tracking=1.2)
        chips.append(f'<rect x="{cx:.1f}" y="371" width="{w + 28:.1f}" height="26" rx="6" class="chip"/><path d="{path}" class="chiptext"/>')
        cx += w + 38

    # Week grid on the right: Mo–Fr columns, 8 time rows.
    gx0, gy0, cw, rh = 896, 86, 64, 34
    days = ["MO", "DI", "MI", "DO", "FR"]
    rnd = random.Random(11)
    cells, labels = [], []
    for c, d in enumerate(days):
        lp, _ = mono.path(d, 11, gx0 + c * cw + 22, gy0 - 22, tracking=1.5, anchor="middle")
        labels.append(f'<path d="{lp}" class="dim"/>')
        for r in range(8):
            x, y = gx0 + c * cw + 22, gy0 + r * rh
            busy = rnd.random() < 0.38
            cells.append(f'<circle cx="{x}" cy="{y}" r="{3.2 if busy else 1.6}" class="{"busy" if busy else "dot"}"/>')
    # Two deadlines that light up.
    dues = [(3, 2), (1, 6)]
    for i, (c, r) in enumerate(dues):
        x, y = gx0 + c * cw + 22, gy0 + r * rh
        cells.append(f'<circle cx="{x}" cy="{y}" r="15" fill="url(#due)" class="pulse" style="animation-delay:{i * 1.4:.1f}s"/>'
                     f'<circle cx="{x}" cy="{y}" r="4.6" fill="{p["due"]}"/>')
    # CIS and Moodle streams merging into one node.
    node = (gx0 + 2 * cw + 22, gy0 + 9 * rh - 4)
    cis = f"M{gx0 - 34} {gy0 + 18} C {gx0 - 10} {gy0 + 150}, {node[0] - 110} {node[1] - 80}, {node[0]} {node[1]}"
    moodle = f"M{gx0 + 5 * cw + 26} {gy0 + 18} C {gx0 + 5 * cw + 6} {gy0 + 150}, {node[0] + 110} {node[1] - 80}, {node[0]} {node[1]}"
    l_cis, _ = mono.path("CIS", 11, gx0 - 34, gy0 + 4, tracking=1.5, anchor="middle")
    l_moo, _ = mono.path("MOODLE", 11, gx0 + 5 * cw + 26, gy0 + 4, tracking=1.5, anchor="middle")
    l_nak, _ = mono.path("nak_deadlines", 12, node[0] + 16, node[1] + 4.5, tracking=0.6)

    return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}" role="img" aria-label="nak-api — CIS and Moodle of the NORDAKADEMIE as one MCP server">
  <title>nak-api</title>
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{p['bg0']}"/><stop offset="1" stop-color="{p['bg1']}"/></linearGradient>
    <radialGradient id="glow" cx=".76" cy=".42" r=".5"><stop offset="0" stop-color="{p['cis']}" stop-opacity="{.20 if dark else .16}"/><stop offset="1" stop-color="{p['cis']}" stop-opacity="0"/></radialGradient>
    <radialGradient id="warm" cx=".98" cy=".05" r=".4"><stop offset="0" stop-color="{p['moodle']}" stop-opacity="{.22 if dark else .15}"/><stop offset="1" stop-color="{p['moodle']}" stop-opacity="0"/></radialGradient>
    <radialGradient id="due"><stop offset="0" stop-color="{p['due']}" stop-opacity=".55"/><stop offset="1" stop-color="{p['due']}" stop-opacity="0"/></radialGradient>
    <linearGradient id="word" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="{p['ink']}"/><stop offset="1" stop-color="{p['cis']}"/></linearGradient>
    <clipPath id="frame"><rect width="{W}" height="{H}" rx="20"/></clipPath>
  </defs>
  <style>
    .dot {{ fill: {p['grid']}; opacity: .28; }}
    .busy {{ fill: {p['grid']}; opacity: .62; }}
    .dim {{ fill: {p['dim']}; }}
    .tag {{ fill: {p['text']}; }}
    .chip {{ fill: {p['grid']}; fill-opacity: {.09 if dark else .07}; stroke: {p['grid']}; stroke-opacity: .30; }}
    .chiptext {{ fill: {p['grid']}; }}
    .stream {{ fill: none; stroke-width: 2.2; stroke-linecap: round; stroke-dasharray: 900; animation: draw 2.6s .3s cubic-bezier(.6,0,.2,1) backwards; }}
    @keyframes draw {{ from {{ stroke-dashoffset: 900; }} to {{ stroke-dashoffset: 0; }} }}
    .pulse {{ animation: pulse 3.2s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }}
    @keyframes pulse {{ 0%, 100% {{ opacity: .35; transform: scale(.75); }} 50% {{ opacity: 1; transform: scale(1.15); }} }}
    .rise {{ animation: rise 1s cubic-bezier(.2,.8,.2,1) backwards; }}
    @keyframes rise {{ from {{ opacity: 0; transform: translateY(12px); }} to {{ opacity: 1; transform: none; }} }}
  </style>
  <g clip-path="url(#frame)">
    <rect width="{W}" height="{H}" fill="url(#bg)"/>
    <rect width="{W}" height="{H}" fill="url(#glow)"/>
    <rect width="{W}" height="{H}" fill="url(#warm)"/>
    {''.join(labels)}
    {''.join(cells)}
    <path d="{cis}" class="stream" stroke="{p['cis']}"/>
    <path d="{moodle}" class="stream" stroke="{p['moodle']}" style="animation-delay:.6s"/>
    <circle cx="{node[0]}" cy="{node[1]}" r="9" fill="{p['bg0']}" stroke="{p['ink']}" stroke-width="2"/>
    <circle cx="{node[0]}" cy="{node[1]}" r="3.4" fill="{p['due']}"/>
    <path d="{l_cis}" fill="{p['cis']}"/><path d="{l_moo}" fill="{p['moodle']}"/><path d="{l_nak}" class="dim"/>
    <g class="rise"><path d="{word}" fill="{p['ink']}"/><path d="{api}" fill="url(#word)" opacity=".9"/></g>
    <g class="rise" style="animation-delay:.25s"><path d="{tag1}" class="tag"/><path d="{tag2}" class="tag"/><path d="{tag3}" class="tag"/></g>
    <g class="rise" style="animation-delay:.45s">{''.join(chips)}</g>
  </g>
  <rect x=".5" y=".5" width="{W - 1}" height="{H - 1}" rx="19.5" fill="none" stroke="{p['grid']}" stroke-opacity=".2"/>
</svg>
"""


def heading(text, kicker, dark):
    W, H = 1280, 116
    p = palette(dark)
    g = Face(font_file("grotesk", 650))
    mono = Face(font_file("mono"))
    k, kw = mono.path(kicker.upper(), 12.5, 0, 30, tracking=2.2)
    t, tw = g.path(text, 52, 0, 92, tracking=-1)
    return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}" role="img" aria-label="{text}">
  <title>{text}</title>
  <defs><linearGradient id="hair" x1="0" x2="1"><stop offset="0" stop-color="{p['cis']}" stop-opacity=".75"/><stop offset=".6" stop-color="{p['moodle']}" stop-opacity=".4"/><stop offset="1" stop-color="{p['moodle']}" stop-opacity="0"/></linearGradient></defs>
  <path d="{k}" fill="{p['cis']}"/>
  <path d="M{kw + 14:.0f} 25.5 h36" stroke="{p['dim']}" stroke-opacity=".5"/>
  <path d="{t}" fill="{p['ink']}"/>
  <rect x="{tw + 26:.0f}" y="74" width="{W - tw - 26:.0f}" height="1.2" fill="url(#hair)"/>
  <rect x="{tw + 23:.0f}" y="71.6" width="6" height="6" rx="1.5" fill="{p['due']}"/>
</svg>
"""


def _row(day, time, src, what, info):
    return f"{day:<11}{time:<8}{src:<8}{what:<32}{info}"


TERMINAL = [
    ("prompt", "$ nak tool nak_deadlines days=14"),
    ("", ""),
    ("due", _row("Mo 12.10.", "23:59", "Moodle", "Abgabe Projektskizze", "I151 Softwaretechnik")),
    ("due", _row("Di 13.10.", "11:30", "CIS", "Klausur Datenbanksysteme", "angemeldet · Abmeldung bis So 11.10.")),
    ("warn", _row("Mi 14.10.", "23:59", "CIS", "Anmeldeschluss Controlling", "nicht angemeldet")),
    ("plain", _row("Fr 16.10.", "16:30", "CIS", "Seminar Führung & Verantw.", "Teilnehmer")),
    ("plain", _row("So 18.10.", "22:59", "Moodle", "Test Foliensatz 9", "I201 Algorithmen")),
    ("plain", _row("Fr 23.10.", "23:59", "CIS", "Abgabe Transferleistung 5", "Beispiel GmbH")),
    ("", ""),
    ("prompt", "$ nak tool cis_klausur_action exam_id=12345 action=register"),
    ("ok", '  "mode": "preview",  "action": "Prüfungsanmeldung",'),
    ("ok", '  "next": "NICHTS wurde gesendet. Nur nach ausdrücklicher Zustimmung mit confirm=true."'),
]


def terminal(dark):
    W, lh = 1280, 30
    H = 92 + lh * len(TERMINAL) + 28
    p = palette(dark)
    mono = Face(font_file("mono"))
    rows = []
    for i, (kind, text) in enumerate(TERMINAL):
        if not text:
            continue
        y = 92 + i * lh
        color = {"prompt": p["ink"], "due": p["due"], "warn": p["moodle"], "ok": p["ok"]}.get(kind, p["text"])
        path, _ = mono.path(text, 17, 44, y)
        rows.append(f'<path d="{path}" fill="{color}" class="line" style="animation-delay:{i * 0.12:.2f}s"/>')
    title, _ = mono.path("nak — zsh", 13, W / 2, 34, anchor="middle")
    return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}" role="img" aria-label="Terminal: nak_deadlines lists deadlines from CIS and Moodle; a binding action only returns a preview">
  <title>nak in the terminal (made-up data)</title>
  <style>.line {{ animation: in .5s ease backwards; }} @keyframes in {{ from {{ opacity: 0; }} to {{ opacity: 1; }} }}</style>
  <rect x=".5" y=".5" width="{W - 1}" height="{H - 1}" rx="16" fill="{p['card']}" stroke="{p['grid']}" stroke-opacity=".25"/>
  <path d="M1 54 H{W - 1}" stroke="{p['grid']}" stroke-opacity=".18"/>
  <circle cx="30" cy="28" r="6" fill="#ff5f57"/><circle cx="52" cy="28" r="6" fill="#febc2e"/><circle cx="74" cy="28" r="6" fill="#28c840"/>
  <path d="{title}" fill="{p['dim']}"/>
  {''.join(rows)}
</svg>
"""


SECTIONS = [
    ("overview", "Everything in one call", "nak_* · both systems"),
    ("study", "Your studies, read properly", "CIS"),
    ("moodle", "Moodle, without the clicking", "moodle_*"),
    ("portal", "naknak, in the browser", "Web portal · self-hosted"),
    ("safety", "Nothing binding without your yes", "Safety"),
    ("install", "Get it", "One binary"),
    ("inside", "Under the hood", "How it works"),
]


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for dark in (True, False):
        mode = "dark" if dark else "light"
        (OUT / f"hero-{mode}.svg").write_text(hero(dark))
        (OUT / f"terminal-{mode}.svg").write_text(terminal(dark))
        for key, title, sub in SECTIONS:
            (OUT / f"h-{key}-{mode}.svg").write_text(heading(title, sub, dark))
    print("written to", OUT)


if __name__ == "__main__":
    main()
