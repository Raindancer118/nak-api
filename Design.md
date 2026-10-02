# nak web — Design

Konzept **Leitstand, leicht**: ein Dashboard, das in fünf Sekunden sagt, was ansteht — luftig, wenige Kacheln, große Zahlen, Bewegung, die Zustand erklärt. Fortsetzung der README-Identität.

## Referenzen

| Quelle | Übernommen |
|---|---|
| nak README (eigene Identität) | Farbcodierung CIS = Blau, Moodle = Orange, fällig = Gelb, erledigt = Grün; Space Grotesk; Nachtblau / warmes Papier |
| Dafi (Behance, Lern-App) | weiche, freundliche Flächen, große Grotesk, kleine Tags |
| Agenda-Shots (Dribbble) | Woche als ruhige Liste mit großen Tagesnamen |

## Farben

| Token | Hell | Dunkel | Zweck |
|---|---|---|---|
| `--bg` | `#f6f4ee` | `#07101f` | Grund |
| `--tile` | `#ffffff` | `#0c172b` | Kacheln |
| `--ink` | `#0d1b33` | `#f2f5fb` | Überschriften, große Zahlen |
| `--text` | `#36445f` | `#b8c4da` | Text |
| `--dim` | `#7b8499` | `#6c7a96` | Nebeninfo |
| `--line` | `#0d1b3314` | `#f2f5fb14` | Haarlinien |
| `--cis` | `#1f63c4` | `#6fb2ff` | Quelle CIS |
| `--moodle` | `#c8601a` | `#f39a4c` | Quelle Moodle |
| `--due` | `#b87a00` | `#ffc857` | dringend (< 72 h) |
| `--ok` | `#1f8a55` | `#7ed4a6` | bestanden |
| `--bad` | `#c23a3a` | `#ff8a80` | nicht bestanden |

Modus folgt `prefers-color-scheme`; Umschalter System → Hell → Dunkel, gespeichert in `localStorage`.

## Typografie

Nur **Space Grotesk** (variabel, lokal, OFL). Zahlen `tabular-nums`.
Große Zahl 600 / `clamp(2.4rem, 5vw, 3.4rem)` / −0.04em · Kachel-Titel 500 / 0.8rem / +0.04em, `--dim` · Text 400 / 0.95rem / 1.5.

## Layout

- Max. 76rem, Raster 12 Spalten, Abstand 1rem; mobil eine Spalte.
- Kopf: Wortmarke, Navigation (Übersicht · Woche · Noten · Module), Stand-Anzeige + Aktualisieren, Modus-Umschalter.
- **Übersicht**: Als Nächstes (breit) · Fristen · Woche · Noten · Prüfungen · Moodle · Fußzeile mit Quartal und Kopierguthaben.
- **Woche**: 14 Tage, Tagesname groß und klebend, Zeilen mit Zeit, Titel, Raum, Quelle.
- **Noten**: Schnitt, Credits-Balken, Tabelle mit Filter.
- **Module**: Liste aus CIS-Noten und Moodle-Kursen, Klick öffnet Detail (`nak_module`).

## Komponenten

- **Kachel**: Radius 18 px, 1 px Linie, im Hellmodus weicher Schatten; kein Cursor-Licht.
- **Quell-Punkt**: 8 px in Quellfarbe vor jeder Zeile.
- **Tag**: Text in Farbe auf 14 % Tönung, Radius 6 px.
- **Tage-Chip** bei Fristen: „heute“, „morgen“, „in 3 T.“ — gelb unter 72 h.
- **Balken**: 6 px, Radius voll, füllt sich animiert.

## Motion

1. Kacheln erscheinen gestaffelt (60 ms): `opacity`, `translate 0 12px`, `scale .985`, Blur 6 px → 0, Feder-Easing.
2. Große Zahlen zählen hoch (`@property` + Counter bzw. JS bei Dezimalzahlen).
3. Balken füllen sich (`scale` X von 0).
4. Laden: Skelett-Schimmer statt Spinner (Daten brauchen bis zu 3 s).
6. Seitenwechsel per View Transition; der Seitentitel morpht.
7. Moduswechsel: kreisförmige Enthüllung vom Umschalter aus.
8. Woche: Zeilen blenden beim Scrollen ein (`animation-timeline: view()`).

`prefers-reduced-motion: reduce`: alles aus, Endzustände sofort.

## Nicht-Ziele

Keine Diagramm-Bibliothek, kein Framework, keine externen Requests, kein Inline-Style (CSP).
