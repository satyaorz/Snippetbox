# Snippetbox
Snippetbox is a simple pastebin application written in Go. It allows users to create and share code snippets with others.

Built following *Let's Go* by Alex Edwards: MySQL-backed models, server-rendered
templates, sessions, user authentication, CSRF protection, HTTPS and an embedded
UI, so the compiled binary is self-contained.

## Requirements
- Go 1.24+
- MySQL 8+

## Setup
```bash
# 1. Create the database, tables and the `web` application user
mysql -uroot < db/schema.sql

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

The server reads `./tls/cert.pem` and `./tls/key.pem`, so run it from the
project root. Templates and static files are embedded in the binary.

## Routing requests
Route pattern              | Handler           | Action
-------------------------- | ----------------- | ------
`GET /{$}`                 | home              | Display the home page
`GET /snippet/view/{id}`   | snippetView       | Display a specific snippet
`GET /snippet/create`      | snippetCreate     | Display a form for creating a new snippet 🔒
`POST /snippet/create`     | snippetCreatePost | Save a new snippet 🔒
`GET /user/signup`         | userSignup        | Display the signup form
`POST /user/signup`        | userSignupPost    | Create a new user
`GET /user/login`          | userLogin         | Display the login form
`POST /user/login`         | userLoginPost     | Authenticate and log in the user
`POST /user/logout`        | userLogoutPost    | Log out the user 🔒
`GET /ping`                | ping              | Health check (returns `OK`)
`GET /static/`             | http.FileServerFS | Serve a specific static file

🔒 requires authentication.

## Middleware
- **standard** (every request): `recoverPanic` → `logRequest` → `commonHeaders`
- **dynamic** (app routes): session `LoadAndSave` → `noSurf` (CSRF) → `authenticate`
- **protected**: dynamic + `requireAuthentication`

## Project layout
```
cmd/web/            HTTP handlers, routes, middleware, templates, main
internal/models/    MySQL models (snippets, users), interfaces, mocks, testdata
internal/validator/ form validation helpers
internal/assert/    test assertion helpers
ui/                 HTML templates and static assets (embedded via ui.Files)
db/                 schema and test-database setup SQL
```

## Testing
```bash
# Unit + end-to-end handler tests (no database needed)
go test -short ./...

# Including MySQL integration tests: create the test DB/user once first
mysql -uroot < db/test_setup.sql
go test ./...

# Coverage
go test -cover ./...
```
