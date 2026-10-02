<img alt="nak-api — CIS and Moodle of the NORDAKADEMIE as one MCP server" src="docs/readme/hero-light.svg#gh-light-mode-only" width="100%">
<img alt="nak-api — CIS and Moodle of the NORDAKADEMIE as one MCP server" src="docs/readme/hero-dark.svg#gh-dark-mode-only" width="100%">

<p align="center">
  <a href="https://github.com/Raindancer118/nak-api/releases"><img alt="release" src="https://img.shields.io/github/v/release/Raindancer118/nak-api?style=for-the-badge&labelColor=07101f&color=6fb2ff&label=release"></a>
  <a href="https://github.com/Raindancer118/nak-api/actions"><img alt="ci" src="https://img.shields.io/github/actions/workflow/status/Raindancer118/nak-api/ci.yml?branch=main&style=for-the-badge&labelColor=07101f&color=7ed4a6&label=ci"></a>
  <img alt="Go" src="https://img.shields.io/github/go-mod/go-version/Raindancer118/nak-api?style=for-the-badge&labelColor=07101f&color=6fb2ff&logo=go&logoColor=6fb2ff">
  <img alt="MCP" src="https://img.shields.io/badge/mcp-83_tools-f39a4c?style=for-the-badge&labelColor=07101f">
  <a href="LICENSE"><img alt="WTFPL" src="https://img.shields.io/badge/license-WTFPL-ffc857?style=for-the-badge&labelColor=07101f"></a>
</p>

<p align="center">
  <b>Ask what's due this week. Get CIS and Moodle in one answer.</b><br>
  Grades with the full Notenspiegel, exams with their real deadlines, your timetable, seminars, electives,<br>
  Transferleistungen and every Moodle course — for any MCP client and on the command line.
</p>

<p align="center">
  <a href="#overview">Overview</a> &nbsp;·&nbsp;
  <a href="#studies">CIS</a> &nbsp;·&nbsp;
  <a href="#moodle">Moodle</a> &nbsp;·&nbsp;
  <a href="#portal">Web portal</a> &nbsp;·&nbsp;
  <a href="#safety">Safety</a> &nbsp;·&nbsp;
  <a href="#get-it">Install</a> &nbsp;·&nbsp;
  <a href="#under-the-hood">How it works</a>
</p>

<br>

<img alt="nak in the terminal: nak_deadlines lists deadlines from CIS and Moodle; a binding action only returns a preview (made-up data)" src="docs/readme/terminal-light.svg#gh-light-mode-only" width="100%">
<img alt="nak in the terminal: nak_deadlines lists deadlines from CIS and Moodle; a binding action only returns a preview (made-up data)" src="docs/readme/terminal-dark.svg#gh-dark-mode-only" width="100%">

<p align="center"><sub>All names, modules and dates in the pictures are made up.</sub></p>

<br>

<a id="overview"></a>
<img alt="Everything in one call" src="docs/readme/h-overview-light.svg#gh-light-mode-only" width="100%">
<img alt="Everything in one call" src="docs/readme/h-overview-dark.svg#gh-dark-mode-only" width="100%">

<table>
<tr>
<td width="50%" valign="top">

**`nak_dashboard`** · today and tomorrow at a glance: lectures, urgent deadlines from both systems, the exams you're registered for, unread Moodle messages, the current quarter and your copy balance.

**`nak_deadlines`** · one chronological list: Moodle submissions and tests, exam dates, the last day to sign up for an exam you're *not* registered for, the last day to withdraw from one you are, Transferleistung due dates and graduation deadlines. Anything under 72 hours is flagged.

</td>
<td width="50%" valign="top">

**`nak_agenda`** · a calendar for any range: lectures from your Zenturie plan (only *your* electives), Moodle due dates and your exams, grouped by day.

**`nak_module I151`** · one module across both systems: grade, attempt and status, the exam and whether you're registered, its place in the study plan, the next sessions, the grade distribution, and the matching Moodle course with its open assignments.

</td>
</tr>
</table>

> [!TIP]
> If one system is down, the others still answer. Results then carry an `unavailable_sources` field instead of failing as a whole.

<br>

<a id="studies"></a>
<img alt="Your studies, read properly" src="docs/readme/h-study-light.svg#gh-light-mode-only" width="100%">
<img alt="Your studies, read properly" src="docs/readme/h-study-dark.svg#gh-dark-mode-only" width="100%">

The CIS has no API. nak reads its TYPO3 pages the way a browser does and turns them into clean data.

<table>
<tr>
<td width="33%" valign="top">

**Grades**
- every module with grade, attempt (`5,0 (2.Versuch)`), status and credits
- seminars, Transferleistungen, average, credits total
- **Notenspiegel** of any exam with fail rate and *your* rank
- attendance per session
- transcript as PDF

</td>
<td width="33%" valign="top">

**Planning**
- study plan: modules × semesters, exam forms, credits
- **progress**: credits earned of 210, what's missing for the Bachelorthesis
- timetable as real events, filtered to your electives
- lecture periods, graduation deadlines

</td>
<td width="33%" valign="top">

**Doing things** ⚠
- register / withdraw from exams, with the PVO deadlines computed
- seminars per quarter, details, seats taken, sign-up and waitlist
- electives: browse, details, choose a Termin
- apply for a Transferleistung (PDF upload)
- profile: contact, address, sharing settings

</td>
</tr>
</table>

<details>
<summary><b>All CIS tools</b></summary>

| Tool | What it does |
| --- | --- |
| `cis_status` · `cis_login` · `cis_logout` | who you are; session handling (renews itself) |
| `cis_profile` · `cis_contact` · `cis_address` | personal data, e-mail/phones, main or term address |
| `cis_sharing` · `cis_payment` · `cis_balance` | grade/registration sharing with your company and classmates, SEPA status (masked), copy balance |
| `cis_classmates` | a Zenturie: names and companies only |
| `cis_grades` · `cis_grade_distribution` · `cis_attendance` · `cis_transcript` | Leistungsübersicht, Notenspiegel + rank, attendance, PDF |
| `cis_studienplan` · `cis_progress` | study plan, credits and thesis prerequisites |
| `cis_timetable` · `cis_list_stundenplan` · `cis_download_stundenplan` | events from the Zenturie calendar, raw .ics/.html files |
| `cis_vorlesungszeiten` · `cis_abschlussfristen` | quarters, graduation deadlines |
| `cis_list_klausuren` · `cis_klausur_action` ⚠ | exam overview with deadlines; register / withdraw |
| `cis_list_seminars` · `cis_seminar_detail` · `cis_seminar_participation` · `cis_seminar_action` ⚠ | seminar programme, details, seats, sign-up / waitlist |
| `cis_list_wahlpflicht` · `cis_wahlpflicht_detail` · `cis_select_wahlpflicht` ⚠ | electives |
| `cis_list_transfer` · `cis_transfer_bewertung` · `cis_download_transfer_document` · `cis_transfer_options` · `cis_transfer_register` ⚠ | Transferleistungen with weighted overall mark |
| `cis_update_contact` ⚠ · `cis_update_address` ⚠ · `cis_set_sharing` ⚠ · `cis_vertiefung` · `cis_set_vertiefung` ⚠ | profile changes |
| `cis_list_certs` · `cis_download_cert` | enrolment certificates (de/en) |
| `cis_page` · `cis_download` | any CIS information page as text plus its downloads |

</details>

<br>

<a id="moodle"></a>
<img alt="Moodle, without the clicking" src="docs/readme/h-moodle-light.svg#gh-light-mode-only" width="100%">
<img alt="Moodle, without the clicking" src="docs/readme/h-moodle-dark.svg#gh-dark-mode-only" width="100%">

<table>
<tr>
<td width="50%" valign="top">

- **Courses and contents** with every file, and **find** across all courses ("foliensatz 9", "klausur pdf")
- **Read files directly**: PDFs page by page, HTML pages, text
- **Assignments** with intro, attachments, your submission status, grade and feedback
- **What's new** since last week in your running courses

</td>
<td width="50%" valign="top">

- **Quizzes** with time windows, your attempts and best grade; **reviews** of finished attempts to learn from
- **Forums and messages**, grades per course, completion, choices and group choices
- **Submit** files or online text ⚠, reply and post ⚠, message ⚠, vote ⚠
- `moodle_call` for any other web service function (writes only with `confirm=true`)

</td>
</tr>
</table>

<br>

<a id="portal"></a>
<img alt="naknak, in the browser" src="docs/readme/h-portal-light.svg#gh-light-mode-only" width="100%">
<img alt="naknak, in the browser" src="docs/readme/h-portal-dark.svg#gh-dark-mode-only" width="100%">

`nak serve` turns the same tools into **naknak**, a web portal you host yourself: CIS and Moodle on one screen, so you never have to open Moodle again. One person per instance — your instance, your data, your server.

<img alt="naknak overview: next lecture, deadlines, week, grades (made-up data)" src="docs/readme/portal/overview-light.webp#gh-light-mode-only" width="100%">
<img alt="naknak overview: next lecture, deadlines, week, grades (made-up data)" src="docs/readme/portal/overview-dark.webp#gh-dark-mode-only" width="100%">

<table>
<tr>
<td width="50%" valign="top">

**One page per module.** Grade, exam history (naknak remembers exams after the CIS forgets them), study plan, dates, lecturers, every linked Moodle course with its content, forum and assignments — matched across number schemes (`A222,I222` for the module graded as `I168`), tutorials and `2`/`II` spellings.

**Moodle inside.** Course content, files (opened in the browser), forums with replies, assignments with upload, messages, news.

**Search and Ctrl+K.** One search for modules, courses, files and activities; a command palette for everything else.

**Old exams.** With an [EduVault](https://eduvault4.de) credential in the settings, every module page lists the matching old and practice exams.

</td>
<td width="50%" valign="top">

**Tells you what's new.** A watcher notices new grades, Moodle content, messages and urgent deadlines — bell, browser notification, or a push via [ntfy](https://ntfy.sh) (without details unless you want them).

**Calendar subscription.** Lectures, exams and deadlines as an ICS feed for any calendar app, with reminders.

**Fast without hammering the NAK.** Stale-while-revalidate cache with per-source freshness (messages 1 min … grades 2 h), shared reads, kept across restarts. Installable PWA that works offline.

**Same safety.** Binding actions show a preview first and need a second, explicit click.

</td>
</tr>
</table>

<img alt="naknak module page: grade, exam history, dates, lecturers (made-up data)" src="docs/readme/portal/modul-light.webp#gh-light-mode-only" width="100%">
<img alt="naknak module page: grade, exam history, dates, lecturers (made-up data)" src="docs/readme/portal/modul-dark.webp#gh-dark-mode-only" width="100%">

<p align="center"><sub>Screenshots from <code>nak serve --demo</code> — every name, grade and date is invented.</sub></p>

**Try it without an account:**

```sh
docker run --rm -p 127.0.0.1:8080:8080 ghcr.io/raindancer118/nak-api serve --demo
# open http://localhost:8080
```

**Run your own** (Docker Compose, the file is in this repo):

```sh
curl -O https://raw.githubusercontent.com/Raindancer118/nak-api/main/docker-compose.yml
docker compose up -d
docker compose logs nak      # once: a login link with a fallback access key
```

Open http://localhost:8080 and sign in with your NORDAKADEMIE account. The first account that signs in owns the instance; its login is checked against the CIS and stored in the data volume (`account.json`, readable only by naknak). No `.env` needed — or set `CIS_USER`/`CIS_PASS` there if you prefer.

> [!IMPORTANT]
> The port is bound to `127.0.0.1` on purpose. To reach naknak from elsewhere, put a TLS reverse proxy in front (Caddy, nginx, Traefik …) instead of opening the port. naknak sets `Secure` cookies and HSTS as soon as the proxy sends `X-Forwarded-Proto: https`. Before an instance is reachable by others, set `NAK_OWNER` to your NAK username so nobody else can claim it first, and `NAK_TRUST_PROXY=1` so failed logins are counted per client, not per proxy.

<details>
<summary><b>What is stored where</b></summary>

Everything lives in the data directory (`/data` in the container, `NAK_DATA_DIR` otherwise), files `0600`:

| File | Content |
| --- | --- |
| `account.json` | NAK login (needed for the CIS and Moodle logins) |
| `session.json` | CIS session cookie |
| `settings.json` | EduVault credential, notification settings |
| `web-token`, `calendar-token` | fallback access key, secret of the calendar feed |
| `web-cache.json` | cached tool results (your grades, messages …) |
| `watch.json`, `history.json` | what the watcher has seen; exams and grade steps |
| `writes.log` | every binding action: target and field names, never values |
| `downloads/`, `uploads/` | opened files; uploads for submissions (removed after a day) |

Signing out also clears the data the browser kept for offline use.

</details>

<br>

<a id="safety"></a>
<img alt="Nothing binding without your yes" src="docs/readme/h-safety-light.svg#gh-light-mode-only" width="100%">
<img alt="Nothing binding without your yes" src="docs/readme/h-safety-dark.svg#gh-dark-mode-only" width="100%">

Exam registrations, seminar sign-ups and elective choices are binding. nak treats them that way.

<table>
<tr>
<td width="50%" valign="top">

**Preview first, always.** Every tool marked ⚠ answers *without* `confirm=true` with a preview of exactly what would be sent — and sends nothing. The gate sits in the tool registry, not in each tool, and a test calls every write tool without confirmation and proves that no request leaves.

**Read-only switch.** `NAK_READONLY=1` blocks every write in both HTTP clients before any network I/O.

</td>
<td width="50%" valign="top">

**A trail of everything.** Each write is appended to `~/.config/cis-api/writes.log`: target and field names, never values.

**Typed confirmation on the CLI.** `confirm=true` additionally needs `JA` typed into an interactive terminal.

**Other people's data stays theirs.** Classmates come back as name and company only; seminar lists as head counts.

</td>
</tr>
</table>

<br>

<a id="get-it"></a>
<img alt="Get it" src="docs/readme/h-install-light.svg#gh-light-mode-only" width="100%">
<img alt="Get it" src="docs/readme/h-install-dark.svg#gh-dark-mode-only" width="100%">

Download a binary from the [latest release](https://github.com/Raindancer118/nak-api/releases/latest) (Linux, macOS, Windows), or build it:

```sh
git clone https://github.com/Raindancer118/nak-api && cd nak-api && go build -o nak .
CIS_USER=12345 CIS_PASS='…' ./nak tool nak_dashboard
```

Add it to your MCP client:

```json
{
  "mcpServers": {
    "nak": { "command": "/path/to/nak", "args": ["mcp"], "env": { "CIS_USER": "…", "CIS_PASS": "…" } }
  }
}
```

> [!TIP]
> Keep the password out of config files: start nak through a secret launcher (vault, `pass`, systemd credentials …) that injects `CIS_USER`/`CIS_PASS` into the environment.

<details>
<summary><b>Configuration</b></summary>

| Variable | Default | Meaning |
| --- | --- | --- |
| `CIS_USER`, `CIS_PASS` | – | NAK login (also used for Moodle) |
| `MOODLE_USER`, `MOODLE_PASS` | CIS login | separate Moodle credentials |
| `NAK_READONLY` | off | `1` blocks all writes |
| `NAK_DOWNLOAD_DIR` | `~/Downloads/nak` | default download folder; files are never overwritten |
| `NAK_TZ` | `Europe/Berlin` | time zone of all dates |
| `MOODLE_URL`, `CIS_BASE_URL` | moodle2 / cis.nordakademie.de | other hosts (tests) |
| `NAK_DATA_DIR` | `~/.config/cis-api` | session, settings, cache and downloads in one place |
| `NAK_CLIENT_CACHE_TTL` | `90s` | identical CIS/Moodle reads are shared this long (`0` = off) |
| `NAK_WEB_ADDR` | `127.0.0.1:8080` | `nak serve` listen address (`0.0.0.0:8080` in the image) |
| `NAK_WEB_TOKEN` | generated | fallback access key for the web UI and API (`Authorization: Bearer …`) |
| `NAK_WEB_CACHE_TTL` | per tool | one freshness for all tools instead of the built-in per-tool values |
| `NAK_WEB_UI_DIR` | embedded | serve the UI from a directory (UI development) |
| `NAK_OWNER` | – | only this NAK username may claim a fresh instance (set it before exposing naknak) |
| `NAK_TRUST_PROXY` | off | `1` = take the client address from `X-Forwarded-For` (behind your own reverse proxy) |
| `EDUVAULT_URL`, `EDUVAULT_TOKEN`, `EDUVAULT_MCP_SECRET` | – | EduVault credential (same variables as the EduVault MCP; the web settings win) |

The CIS session lives in `~/.config/cis-api/session.json` (0600) and is renewed automatically when it expires. PDF text uses Poppler's `pdftotext` when installed, a built-in extractor otherwise.

</details>

<details>
<summary><b>Command line</b></summary>

```
nak mcp                          MCP server on stdio
nak tools [filter] [--params]    list tools (WRITE = binding)
nak tool <name> key=value …      run a tool, JSON out; values may be JSON
nak login | logout               CIS session
nak selfcheck [--report] [--json]   is the CIS still as expected? file issues for changes
nak serve [--addr] [--demo]      naknak web portal (--demo: invented data, no account)
nak healthcheck [--url]          exit 0 if the web portal answers (container health check)
```

```sh
nak tool cis_grade_distribution module_nr=I140
nak tool cis_timetable from=2026-10-12 days=5
nak tool cis_list_seminars quarter="2026 - 4" available_only=true
nak tool moodle_find query="foliensatz 9"
nak tool cis_klausur_action exam_id=12345 action=register     # preview only
```

</details>

<br>

<a id="under-the-hood"></a>
<img alt="Under the hood" src="docs/readme/h-inside-light.svg#gh-light-mode-only" width="100%">
<img alt="Under the hood" src="docs/readme/h-inside-dark.svg#gh-dark-mode-only" width="100%">

```mermaid
flowchart TB
    client(["MCP client / CLI"]) -- "stdio" --> reg["tool registry<br/><sub>write gate · confirm</sub>"]
    reg --> cisp["CIS packages<br/><sub>grades · exams · seminars …</sub>"]
    reg --> mdl["Moodle service"]
    reg --> nak["nak_* cross tools"]
    nak --> cisp & mdl
    cisp --> forms["form engine<br/><sub>browser-exact submits</sub>"]
    cisp --> http["CIS client<br/><sub>read · write · audit</sub>"]
    forms --> http
    http -- "HTML (TYPO3)" --> CIS[("cis.nordakademie.de")]
    mdl -- "REST · token" --> Moodle[("moodle2.nordakademie.de")]
```

<details>
<summary><b>How it works, in detail</b></summary>

- **Two paths in the CIS client.** `Page`/`PostRead`/`Download` only read; an expired session (HTTP 403 with the login form) triggers one transparent re-login. `WriteGet`/`WritePost`/`WriteMultipart` are the only way to change anything, honour the read-only switch and are audited. Absolute URLs to other hosts are refused.
- **cHash, never computed.** TYPO3 signs links with a `cHash`. nak follows the links and forms the page itself offers, so it never forges one.
- **Forms like a browser.** The form engine rebuilds submissions exactly: hidden `__trustedProperties`, the hidden twin of every checkbox, only the clicked button, GET forms replacing the action's query. A `Submission` is built and previewed long before anyone may send it.
- **Cells by label, not position.** The grade tables carry `data-label` attributes; parsing by label survives layout changes.
- **Real flows.** Exam actions are signed GET links; the elective choice is the `order` link of a Termin on the detail view; a Transferleistung goes `new → confirmNew → create`; the grade distribution comes from the page's chart data.
- **Deadlines.** Sign-up opens 25 and closes 10 calendar days before a written exam, withdrawal closes 2 days before (PVO §10/§11). Computed per exam; the link the CIS actually shows wins.
- **Moodle** uses the mobile web service: token from `login/token.php` (renewed on `invalidtoken`), Moodle-style parameter flattening, files through `webservice/pluginfile.php`, draft uploads for submissions.

</details>

<details>
<summary><b>Development</b></summary>

```sh
go test -race ./...        # parsers, form engine, clients, MCP end-to-end, binary start-up
go vet ./...
```

- Parser tests run against **anonymised fixtures** in `internal/*/testdata/`: real CIS page structure, made-up people and grades.
- `internal/tools` starts the MCP server against a fake CIS and a fake Moodle, calls every read tool, calls every write tool without confirmation and asserts that nothing was sent, then confirms one and checks the exact request.
- `cmd` builds the real binary and talks JSON-RPC to it over stdio.
- README artwork: `scripts/readme-art.py` (text set with HarfBuzz and stored as outlines).
- Releases: `git tag X.Y.Z && git push origin X.Y.Z` — CI builds five platforms and publishes the GitHub release.

</details>

<br>

### When the CIS changes

The CIS is a website, not an API — sooner or later a page will look different. nak notices instead of returning wrong data:

<table>
<tr>
<td width="50%" valign="top">

**In every answer.** When a parser misses the structure it expects, the tool says so and offers a pre-filled issue link — plus `nak_report_drift`, which files it after you confirm.

**`nak selfcheck`** checks, read-only, every page and form nak depends on (including the ones write actions need) and tells *drift* (the CIS changed) apart from *unavailable* (network, login, maintenance).

</td>
<td width="50%" valign="top">

**`nak selfcheck --report`** opens one GitHub issue per broken check, comments if it's still broken, and closes it once the check passes again. Run it daily, e.g. from a systemd timer.

**No personal data, ever.** Reports contain the page *outline* only: table headers, data-label and field names, CSS classes. Never cell values, headings or names — the repository is public.

</td>
</tr>
</table>

> [!NOTE]
> Something broken and no token at hand? [Open an issue](https://github.com/Raindancer118/nak-api/issues/new?template=cis-change.yml) and paste `nak selfcheck` — no grades or names, please.

### Good to know

- Not an official NORDAKADEMIE project. It uses your own account and only does what you could do in the browser.
- The CIS can change its pages at any time; parsers fail loudly rather than return wrong data, and the fixtures make fixes quick.
- A timetable only exists once the CIS publishes the Zenturie's calendar for that semester; nak says so instead of showing an empty week.

<br>

<p align="center">
  <sub>Successor of <a href="https://github.com/Raindancer118/cis-api">cis-api</a> and <a href="https://github.com/Raindancer118/moodle-mcp">moodle-mcp</a> · <a href="LICENSE">WTFPL</a> · bundled dependencies in <a href="THIRD-PARTY.md">THIRD-PARTY.md</a></sub>
</p>
