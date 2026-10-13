package main

import (
	"bytes"
	"html/template"
	"net/http"
	"sync"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	syntaxStyle = styles.Get("github")

	// WithClasses makes chroma emit CSS class names instead of inline style
	// attributes. That matters because our Content-Security-Policy only
	// allows stylesheets from 'self', so inline styles would be blocked.
	syntaxFormatter = chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.WithLineNumbers(true),
		chromahtml.TabWidth(4),
	)
)

// highlight renders a snippet's content as syntax-highlighted HTML. Chroma
// HTML-escapes every token it writes, so the result is safe to mark as
// template.HTML. If anything goes wrong we fall back to plain escaped text.
func highlight(content, lang string) template.HTML {
	fallback := template.HTML("<pre><code>" + template.HTMLEscapeString(content) + "</code></pre>")

	lexer := lexers.Fallback
	if lang != "" {
		if l := lexers.Get(lang); l != nil {
			lexer = l
		}
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return fallback
	}

	var buf bytes.Buffer
	err = syntaxFormatter.Format(&buf, syntaxStyle, iterator)
	if err != nil {
		return fallback
	}

	return template.HTML(buf.String())
}

// syntaxCSS generates the stylesheet for the highlighter's CSS classes once,
// the first time it's requested.
var syntaxCSS = sync.OnceValues(func() ([]byte, error) {
	var buf bytes.Buffer
	err := syntaxFormatter.WriteCSS(&buf, syntaxStyle)
	return buf.Bytes(), err
})

// syntaxCSSHandler serves the generated highlighting stylesheet.
func (app *application) syntaxCSSHandler(w http.ResponseWriter, r *http.Request) {
	css, err := syntaxCSS()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(css)
}
