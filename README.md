# cis-api

Go CLI client for the [NORDAKADEMIE](https://www.nordakademie.de) Campus Information System (CIS).

Reverse-engineered from the TYPO3-based CIS portal at `cis.nordakademie.de`.

## Features

- **Login / Logout** — TYPO3 felogin with session persistence (`~/.config/cis-api/session.json`)
- **Grades** — fetch Leistungsübersicht / Notenspiegel as a table or JSON
- **Certificates** — list and download Online-Bescheinigungen

## Install

```sh
git clone https://github.com/Raindancer118/cis-api.git
cd cis-api
go build -o cis .
```

Requires Go 1.21+.

## Usage

```sh
# Login (interactive prompt or env vars)
./cis login
CIS_USER=20066 CIS_PASS=geheim ./cis login

# Show grades
./cis grades
./cis grades --lang en
./cis grades --json

# List certificates
./cis certs

# Download certificate #1
./cis certs --download 1 --out ~/Downloads

# Logout
./cis logout
```

## How it works

The CIS runs on **TYPO3 (Extbase)** — there is no REST API. All responses are HTML.

1. **Auth:** GET `/` → extract hidden form fields → POST credentials. Session stored as `fe_typo_user_cae070b` cookie.
2. **cHash:** TYPO3 signs almost every URL with a `cHash` parameter that cannot be computed client-side. The client always extracts pre-signed links from the page HTML and follows those.
3. **Scraping:** HTML tables are parsed with `golang.org/x/net/html` and matched by column header keywords.

## Architecture

```
cmd/          Cobra CLI commands (login, grades, certs)
internal/
  client/     HTTP client with cookie jar + session persistence
  auth/       TYPO3 felogin flow
  scraper/    Generic HTML utilities (forms, links, tables)
  grades/     Grade fetching and parsing
  certs/      Certificate listing and download
```

## Security

- Credentials are never stored on disk — only the session cookie is persisted (mode 0600).
- Credentials can be passed via `CIS_USER` / `CIS_PASS` env vars for scripting.
