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
- Go 1.24+
- MySQL 8+

## Setup
```bash
# 1. Create the database, tables and the `web` application user
mysql -uroot < db/schema.sql

#    Upgrading a database created before Phase 2? Apply the migration instead:
#    mysql -uroot snippetbox < db/migrations/001_phase2.sql

# 2. Generate a self-signed TLS certificate for local development
mkdir -p tls && cd tls
go run "$(go env GOROOT)/src/crypto/tls/generate_cert.go" --rsa-bits=2048 --host=localhost
cd ..

# 3. Run the server (https://localhost:4000)
go run ./cmd/web
```

Flags:

| Flag    | Default                                   | Description           |
| ------- | ----------------------------------------- | --------------------- |
| `-addr` | `:4000`                                   | HTTPS network address |
| `-dsn`  | `web:pass@/snippetbox?parseTime=true`     | MySQL DSN             |
| `-debug`| `false`                                   | Show errors and stack traces in 500 responses (dev only) |
| `-cleanup-interval` | `1h`                          | How often expired snippets are deleted |

The server reads `./tls/cert.pem` and `./tls/key.pem`, so run it from the
project root. Templates and static files are embedded in the binary.

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
- **standard** (every request): `recoverPanic` → `logRequest` → `commonHeaders`
- **dynamic** (app routes): session `LoadAndSave` → `noSurf` (CSRF) → `authenticate`
- **limited**: dynamic + `limitAuthAttempts` (password-checking form submissions)
- **protected**: dynamic + `requireAuthentication`

`authenticate` also compares the session's `sessionVersion` with the user's
`session_version` column, which is bumped on every password change, so changing
your password logs out your other sessions.

The rate limiter keys on `r.RemoteAddr`. Behind a reverse proxy every request
would share one bucket, so that will need trusted-proxy handling before deploying.

## Project layout
```
cmd/web/            HTTP handlers, routes, middleware, templates, main
internal/models/    MySQL models (snippets, users), interfaces, mocks, testdata
internal/validator/ form validation helpers
internal/assert/    test assertion helpers
ui/                 HTML templates and static assets (embedded via ui.Files)
db/                 schema, migrations and test-database setup SQL
```

## Testing
```bash
# Unit + end-to-end handler tests (no database needed)
go test -short ./...

# Including MySQL integration tests: create the test DB/user once first
# (re-run it if you set it up before Phase 2 — the user now needs REFERENCES)
mysql -uroot < db/test_setup.sql
go test ./...

# Coverage
go test -cover ./...
```
