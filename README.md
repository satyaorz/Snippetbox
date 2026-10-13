# Snippetbox
Snippetbox is a simple pastebin application written in Go. It allows users to create and share code snippets with others.

Built following *Let's Go* by Alex Edwards: MySQL-backed models, server-rendered
templates, sessions, user authentication, CSRF protection, HTTPS and an embedded
UI, so the compiled binary is self-contained.

## Features
- Syntax highlighting for 20 languages (server-side, via chroma) and a raw text view
- Snippets belong to their author, who can edit or delete them
- Public, unlisted (link-only) and private (owner-only) visibility
- Random 12-character URLs (`/snippet/view/3f27b2f6963c`), so snippets can't be enumerated
- Full-text search and pagination on the home page
- Accounts: signup, login, account page with your snippets, change password
- Changing your password logs out your other sessions
- Rate limiting on login, signup and change password (per IP)
- Expired snippets are deleted automatically in the background

## Requirements
- Go 1.26+
- MySQL 8+
- Docker (optional, for the compose stack)

## Quick start with Docker
```bash
docker compose up --build
```
Then open https://localhost:8443. The stack runs MySQL, applies migrations,
starts the app behind [Caddy](https://caddyserver.com) (which terminates TLS
with its own local CA, so your browser will warn until you trust it) and
exposes the private metrics server on http://127.0.0.1:4001.

## Local setup
```bash
# 1. Create the database and the read/write-only `web` user
mysql -uroot < db/setup.sql

# 2. Create the tables (needs an account that can change the schema)
go run ./cmd/migrate -dsn 'root@/snippetbox' up

# 3. Generate a self-signed TLS certificate for local development
mkdir -p tls && cd tls
go run "$(go env GOROOT)/src/crypto/tls/generate_cert.go" --rsa-bits=2048 --host=localhost
cd ..

# 4. Run the server (https://localhost:4000)
go run ./cmd/web
```

## Migrations
Schema changes live in `db/migrations/NNNN_description.sql` and are embedded
in the `migrate` binary, which records what it has applied in a
`schema_migrations` table (and takes a MySQL lock so two deploys can't migrate
at once).

```bash
go run ./cmd/migrate up                # apply pending migrations
go run ./cmd/migrate status            # list migrations
go run ./cmd/migrate baseline 2        # mark 0001-0002 as applied without running them
```

Databases created before migrations were tracked (from the old
`db/schema.sql`, or upgraded by hand with the Phase 2 script) should run
`baseline 2` once. The DSN can also come from `SNIPPETBOX_MIGRATE_DSN`.

## Configuration
Every flag can also be set with an environment variable: `-cleanup-interval`
becomes `SNIPPETBOX_CLEANUP_INTERVAL`. Command-line flags win.

| Flag                  | Default                               | Description |
| --------------------- | ------------------------------------- | ----------- |
| `-addr`               | `:4000`                               | Network address for the app |
| `-dsn`                | `web:pass@/snippetbox?parseTime=true` | MySQL DSN |
| `-tls`                | `file`                                | `file` (cert/key files), `autocert` (Let's Encrypt) or `off` (plain HTTP behind a proxy) |
| `-tls-cert`, `-tls-key` | `./tls/cert.pem`, `./tls/key.pem`   | Certificate files for `-tls=file` |
| `-autocert-domains`   |                                       | Comma-separated domains for `-tls=autocert` |
| `-autocert-email`     |                                       | Let's Encrypt contact email |
| `-autocert-cache`     | `./autocert-cache`                    | Where certificates are cached |
| `-autocert-http-addr` | `:80`                                 | ACME challenge / HTTPS redirect server |
| `-behind-proxy`       | `false`                               | Trust `X-Forwarded-For`, `X-Forwarded-Proto` and `X-Request-Id` from a reverse proxy |
| `-log-format`         | `text`                                | `text` or `json` |
| `-log-level`          | `info`                                | `debug`, `info`, `warn` or `error` |
| `-metrics-addr`       |                                       | Private server for `/debug/vars` and `/healthz` (off if empty) |
| `-cleanup-interval`   | `1h`                                  | How often expired snippets are deleted |
| `-shutdown-timeout`   | `20s`                                 | Grace period for in-flight requests on SIGINT/SIGTERM |
| `-debug`              | `false`                               | Show errors and stack traces in 500 responses (dev only) |
| `-version`            |                                       | Print the build's git revision and exit |

**Deploying.** Either run the app directly on ports 443/80 with
`-tls=autocert -autocert-domains=example.com -addr=:443`, or put it behind a
reverse proxy with `-tls=off -behind-proxy` (as `compose.yaml` does). Only use
`-behind-proxy` when the proxy is the only way to reach the app, since it
trusts the proxy's headers.

**Observability.** Every request is logged once it completes (status, size,
duration) with a request ID that's also returned in the `X-Request-Id`
header. With `-metrics-addr` set, `/debug/vars` exposes request counts by
status, processing time, goroutines and database pool stats, and `/healthz`
checks the database connection. Keep that address private.

## Routing requests
Route pattern                   | Handler                   | Action
------------------------------- | ------------------------- | ------
`GET /{$}`                      | home                      | Latest snippets; `?q=` searches, `?page=` paginates
`GET /about`                    | about                     | Display the about page
`GET /snippet/view/{slug}`      | snippetView               | Display a specific snippet
`GET /snippet/raw/{slug}`       | snippetRaw                | The snippet's content as plain text
`GET /snippet/create`           | snippetCreate             | Display a form for creating a new snippet 🔒
`POST /snippet/create`          | snippetCreatePost         | Save a new snippet 🔒
`GET /snippet/edit/{slug}`      | snippetEdit               | Display the edit form (owner only) 🔒
`POST /snippet/edit/{slug}`     | snippetEditPost           | Save changes to a snippet (owner only) 🔒
`POST /snippet/delete/{slug}`   | snippetDeletePost         | Delete a snippet (owner only) 🔒
`GET /user/signup`              | userSignup                | Display the signup form
`POST /user/signup`             | userSignupPost            | Create a new user ⏱
`GET /user/login`               | userLogin                 | Display the login form
`POST /user/login`              | userLoginPost             | Authenticate and log in the user ⏱
`POST /user/logout`             | userLogoutPost            | Log out the user 🔒
`GET /account/view`             | accountView               | Account details and your snippets 🔒
`GET /account/password/update`  | accountPasswordUpdate     | Display the change password form 🔒
`POST /account/password/update` | accountPasswordUpdatePost | Change the user's password 🔒 ⏱
`GET /ping`                     | ping                      | Health check (returns `OK`)
`GET /static/css/syntax.css`    | syntaxCSSHandler          | Generated syntax highlighting stylesheet
`GET /static/`                  | http.FileServerFS         | Serve a specific static file

🔒 requires authentication. ⏱ rate limited per IP: a burst of 5, then one every 12 seconds.
Private snippets return 404 to everyone except their owner.

## Middleware
- **standard** (every request): `requestID` → `logRequest` → `recoverPanic` → `commonHeaders`
- **dynamic** (app routes): session `LoadAndSave` → `noSurf` (CSRF) → `authenticate`
- **limited**: dynamic + `limitAuthAttempts` (password-checking form submissions)
- **protected**: dynamic + `requireAuthentication`

`authenticate` also compares the session's `sessionVersion` with the user's
`session_version` column, which is bumped on every password change, so changing
your password logs out your other sessions.

The rate limiter keys on the client IP: the TCP peer, or with `-behind-proxy`
the last address in `X-Forwarded-For` (the one the proxy added).

## Project layout
```
cmd/web/            HTTP handlers, routes, middleware, templates, config, servers
cmd/migrate/        schema migration command
internal/models/    MySQL models (snippets, users), interfaces, mocks, testdata
internal/validator/ form validation helpers
internal/migrate/   migration runner used by cmd/migrate and the tests
internal/assert/    test assertion helpers
ui/                 HTML templates and static assets (embedded via ui.Files)
db/                 embedded migrations, database/user setup SQL
deploy/             Caddyfile and MySQL init script for compose.yaml
```

## Testing
```bash
# Unit + end-to-end handler tests (no database needed)
go test -short ./...

# Including MySQL integration tests: create the test DB/user once first
# (re-run it if you set it up before Phase 2 — the user now needs REFERENCES)
mysql -uroot < db/test_setup.sql
go test ./...

# The integration tests build their schema by running the real migrations.
# Point them at another database with SNIPPETBOX_TEST_DSN.

# Coverage
go test -cover ./...
```

CI (GitHub Actions) runs gofmt, `go vet`, the full test suite with `-race`
against a MySQL service, `govulncheck`, and a Docker build on every push and
pull request.
