# cis-api

Go CLI + MCP server for the [NORDAKADEMIE](https://www.nordakademie.de) Campus Information System (CIS).

Reverse-engineered from the TYPO3-based portal at `cis.nordakademie.de`. Works as a standalone CLI and as an MCP server so Claude Code can interact directly with the CIS.

## Features

| | CLI | MCP |
|---|---|---|
| Login / Logout | ✓ | ✓ |
| Grades (Leistungsübersicht) | ✓ | ✓ |
| Stundenplan list + download (.ics/.html per Zenturie) | ✓ | ✓ |
| Klausuren list + register/deregister (binding) | ✓ | ✓ |
| Transferleistungen list + grading (computed Gesamtnote) | ✓ | ✓ |
| Seminar list + details | — | ✓ |
| Wahlpflichtmodul list + details + selection (binding) | ✓ | ✓ |
| Certificate list + download | ✓ | ✓ |

> **Binding write actions** (Klausur an-/abmelden, Wahlpflicht wählen) default to a
> dry run. They only execute with `--confirm` plus an interactive typed confirmation.

## Install

```sh
git clone https://github.com/Raindancer118/cis-api.git
cd cis-api
go build -o cis .
```

Requires Go 1.21+.

## CLI Usage

```sh
# Login (interactive prompt, or via env vars)
./cis login
CIS_USER=20066 CIS_PASS='yourpassword' ./cis login

# Grades
./cis grades
./cis grades --lang en
./cis grades --json

# Stundenplan (timetable calendars per Zenturie)
./cis stundenplan                          # list all
./cis stundenplan -z I24a -f ics           # filter
./cis stundenplan -z I24a -f ics -d -o ~/Downloads   # download the .ics

# Klausuren (exam registration — binding writes need --confirm)
./cis klausuren                            # list exams + examIds
./cis klausuren --register 12022           # dry run
./cis klausuren --register 12022 --confirm # actually register (asks again)

# Transferleistungen
./cis transfer                             # overview
./cis transfer --bewertung 14534           # grading detail + weighted Gesamtnote

# Wahlpflichtmodule
./cis wpf                                  # list
./cis wpf --select 1234 --confirm          # binding selection (asks again)

# Certificates
./cis certs
./cis certs --download 1 --out ~/Downloads

# Logout
./cis logout
```

## MCP Server

Add to your Claude Code `~/.claude/.mcp.json` (create the file if it doesn't exist):

```json
{
  "mcpServers": {
    "cis": {
      "command": "/path/to/cis",
      "args": ["mcp"],
      "env": {
        "CIS_USER": "your_student_id",
        "CIS_PASS": "your_password"
      }
    }
  }
}
```

Or run `cis login` once first — the session is saved to `~/.config/cis-api/session.json` and reused automatically.

### Available MCP Tools

| Tool | Description |
|---|---|
| `cis_login` | Log in with username + password |
| `cis_logout` | Log out and clear session |
| `cis_grades` | Fetch grades (Leistungsübersicht) |
| `cis_list_stundenplan` | List timetable files per Zenturie (.ics/.html) |
| `cis_download_stundenplan` | Download a timetable file to a local path |
| `cis_list_klausuren` | List the exam overview (Prüfungsübersicht) |
| `cis_klausur_action` | Register/deregister for an exam (binding; needs `confirm=true`) |
| `cis_list_transfer` | List Transferleistungen / Praxisberichte |
| `cis_transfer_bewertung` | Grading detail + client-side weighted Gesamtnote |
| `cis_download_transfer_document` | Download a Transferleistung attachment |
| `cis_list_seminars` | List all available seminars with IDs |
| `cis_seminar_detail` | Seminar details (dozent, dates, credits) |
| `cis_list_wahlpflicht` | List Wahlpflichtmodule for your curriculum |
| `cis_wahlpflicht_detail` | Module details + whether selection is open |
| `cis_select_wahlpflicht` | Select a Wahlpflichtmodul (when period is open) |
| `cis_list_certs` | List downloadable certificates |
| `cis_download_cert` | Download a certificate to a local file |

## How it works

The CIS runs on **TYPO3 (Extbase)** — no REST API, all responses are server-rendered HTML.

**Auth:** `GET /` → extract hidden TYPO3 form fields → `POST` credentials. Session stored as `fe_typo_user_cae070b` cookie, persisted in `~/.config/cis-api/session.json` (mode 0600).

**cHash:** TYPO3 signs every URL with a `cHash` parameter that cannot be computed client-side. The client always extracts pre-signed links from page HTML and follows those — making it robust against parameter changes.

**Wahlpflichtmodule selection:** The selection form only appears during the selection period. `cis_wahlpflicht_detail` reports `select_available: true` when the button is live.

## Security

- Credentials are **never stored on disk** — only the session cookie is persisted (mode 0600).
- Pass credentials via `CIS_USER` / `CIS_PASS` env vars for scripting/MCP.
