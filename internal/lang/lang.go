// Package lang lists the languages snippets can be highlighted as. It's
// shared by the web server (forms, validation, highlighting) and the snip
// CLI (detecting a language from a file name).
package lang

import (
	"path/filepath"
	"strings"
)

// Language is one highlighting option. Value is the chroma lexer name stored
// in the database ("" means plain text).
type Language struct {
	Value      string
	Label      string
	Extensions []string
}

// All lists every supported language, plain text first.
var All = []Language{
	{"", "Plain text", []string{".txt"}},
	{"bash", "Shell", []string{".sh", ".bash", ".zsh"}},
	{"c", "C", []string{".c", ".h"}},
	{"cpp", "C++", []string{".cpp", ".cc", ".cxx", ".hpp"}},
	{"csharp", "C#", []string{".cs"}},
	{"css", "CSS", []string{".css"}},
	{"go", "Go", []string{".go"}},
	{"html", "HTML", []string{".html", ".htm"}},
	{"java", "Java", []string{".java"}},
	{"javascript", "JavaScript", []string{".js", ".mjs", ".cjs", ".jsx"}},
	{"json", "JSON", []string{".json"}},
	{"markdown", "Markdown", []string{".md", ".markdown"}},
	{"php", "PHP", []string{".php"}},
	{"python", "Python", []string{".py"}},
	{"ruby", "Ruby", []string{".rb"}},
	{"rust", "Rust", []string{".rs"}},
	{"solidity", "Solidity", []string{".sol"}},
	{"sql", "SQL", []string{".sql"}},
	{"typescript", "TypeScript", []string{".ts", ".tsx"}},
	{"yaml", "YAML", []string{".yaml", ".yml"}},
}

// Values returns the permitted stored values.
func Values() []string {
	values := make([]string, len(All))
	for i, l := range All {
		values[i] = l.Value
	}
	return values
}

// Name returns the human-readable label for a stored value.
func Name(value string) string {
	for _, l := range All {
		if l.Value == value {
			return l.Label
		}
	}
	return "Plain text"
}

// Detect guesses the language from a file name's extension, returning ""
// (plain text) if it doesn't recognise it.
func Detect(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return ""
	}
	for _, l := range All {
		for _, e := range l.Extensions {
			if e == ext {
				return l.Value
			}
		}
	}
	return ""
}
