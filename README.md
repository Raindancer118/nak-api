# nak-api

**One MCP server and CLI for the NORDAKADEMIE: Campus Information System (CIS) + Moodle.**

Successor of [`cis-api`](https://github.com/Raindancer118/cis-api) and [`moodle-mcp`](https://github.com/Raindancer118/moodle-mcp), merged into one Go binary.
The CIS has no API — everything is reverse-engineered from the TYPO3 pages. Moodle is accessed through its mobile web service.

## What it can do

| Area | Tools |
|---|---|
| **Overview** | `nak_dashboard` (today: lectures, urgent deadlines, exams, unread messages) · `nak_deadlines` (CIS + Moodle, chronological) · `nak_agenda` (combined calendar) · `nak_module` (one module across both systems) |
| **Grades** | `cis_grades` (modules, attempts, seminars, Transferleistungen, stats) · `cis_grade_distribution` (Notenspiegel + your rank) · `cis_attendance` · `cis_transcript` (PDF) · `cis_progress` (credits, thesis prerequisites) |
| **Planning** | `cis_timetable` (lectures as events, only your electives) · `cis_studienplan` · `cis_vorlesungszeiten` · `cis_abschlussfristen` · Stundenplan files |
| **Exams** | `cis_list_klausuren` (incl. computed PVO deadlines) · `cis_klausur_action` ⚠ |
| **Seminars** | list per quarter / mine · detail · participation counts · `cis_seminar_action` ⚠ |
| **Electives** | `cis_list_wahlpflicht` · detail · `cis_select_wahlpflicht` ⚠ |
| **Transferleistungen** | list · grading with weighted mark · documents · `cis_transfer_register` ⚠ |
| **Profile** | personal data · contact · address · sharing · SEPA status (read only) · classmates (name + company only) · copy balance · `cis_update_contact` ⚠ `cis_update_address` ⚠ `cis_set_sharing` ⚠ `cis_set_vertiefung` ⚠ |
| **Documents** | enrolment certificates (PDF) · any CIS page as text + downloads (`cis_page`, `cis_download`) |
| **Moodle** | courses, contents, find, files (PDF text), assignments + submissions ⚠, grades, forums ⚠, messages ⚠, quizzes + reviews, choices ⚠, completion, what's new, raw web service calls |

`nak tools` lists all 78 tools.

## Safety

Tools marked ⚠ change data (exam registration, seminar sign-up, elective choice, submissions …).

- Without `confirm=true` they return a **preview** and send nothing — enforced centrally in the tool registry and covered by tests for every write tool.
- `NAK_READONLY=1` blocks every write in both HTTP clients before any network I/O.
- Every write is appended to `~/.config/cis-api/writes.log` (target and field names, never values).
- On the CLI, `confirm=true` additionally requires typing `JA` in an interactive terminal.
- Other students' personal data (birthdays, phone numbers, participant names) is never returned.

## Install

```sh
go install github.com/Raindancer118/nak-api@latest   # binary: nak-api
# or
git clone https://github.com/Raindancer118/nak-api && cd nak-api && go build -o nak .
```

Release binaries (Linux, macOS, Windows) are attached to every [GitHub release](https://github.com/Raindancer118/nak-api/releases).

## Configuration

| Variable | Meaning |
|---|---|
| `CIS_USER`, `CIS_PASS` | NAK login (also used for Moodle) |
| `MOODLE_USER`, `MOODLE_PASS` | optional separate Moodle credentials |
| `NAK_READONLY=1` | block all writes |
| `NAK_DOWNLOAD_DIR` | default download folder (`~/Downloads/nak`) |
| `MOODLE_URL`, `CIS_BASE_URL`, `NAK_TZ` | overrides (defaults: moodle2.nordakademie.de, cis.nordakademie.de, Europe/Berlin) |

The CIS session is cached in `~/.config/cis-api/session.json` (0600) and renewed automatically when it expires.

## Use as MCP server

```json
{
  "mcpServers": {
    "nak": { "command": "/path/to/nak", "args": ["mcp"], "env": { "CIS_USER": "…", "CIS_PASS": "…" } }
  }
}
```

## CLI

```sh
nak login
nak tool nak_dashboard
nak tool cis_grade_distribution module_nr=I140
nak tool cis_timetable from=2026-10-12 days=5
nak tool moodle_find query="foliensatz 9"
nak tool cis_klausur_action exam_id=12345 action=register            # preview only
```

## Development

```sh
go test -race ./...
```

Parser tests run against anonymised fixtures in `internal/*/testdata/` (real CIS structure, fake personal data). The end-to-end tests in `internal/tools` start the MCP server against a fake CIS and a fake Moodle and assert that no write tool sends anything without confirmation.

## License

[WTFPL](LICENSE) — see [THIRD-PARTY.md](THIRD-PARTY.md) for bundled dependencies.
