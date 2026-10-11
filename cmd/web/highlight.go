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

// language is a syntax highlighting option offered on the snippet form. Value
// is the chroma lexer name stored in the database; "" means plain text.
type language struct {
	Value string
	Label string
}

var languages = []language{
	{"", "Plain text"},
	{"bash", "Shell"},
	{"c", "C"},
	{"cpp", "C++"},
	{"csharp", "C#"},
	{"css", "CSS"},
	{"go", "Go"},
	{"html", "HTML"},
	{"java", "Java"},
	{"javascript", "JavaScript"},
	{"json", "JSON"},
	{"markdown", "Markdown"},
	{"php", "PHP"},
	{"python", "Python"},
	{"ruby", "Ruby"},
	{"rust", "Rust"},
	{"solidity", "Solidity"},
	{"sql", "SQL"},
	{"typescript", "TypeScript"},
	{"yaml", "YAML"},
}

// languageValues returns the permitted values for the language form field.
func languageValues() []string {
	values := make([]string, len(languages))
	for i, l := range languages {
		values[i] = l.Value
	}
	return values
}

// languageName returns the human-readable label for a stored language value.
func languageName(value string) string {
	for _, l := range languages {
		if l.Value == value {
			return l.Label
		}
	}
	return "Plain text"
}

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
