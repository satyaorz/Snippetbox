package ui

import "embed"

// Files holds the HTML templates and static assets, embedded into the binary
// at compile time. The paths are relative to this ui directory, so a file is
// accessed as e.g. "html/base.tmpl.html" or "static/css/main.css".
//
//go:embed "html" "static"
var Files embed.FS
