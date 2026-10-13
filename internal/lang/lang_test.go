package lang

import (
	"testing"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestDetect(t *testing.T) {
	tests := map[string]string{
		"main.go":            "go",
		"src/lib.rs":         "rust",
		"Token.SOL":          "solidity",
		"docker-compose.yml": "yaml",
		"notes.txt":          "",
		"Makefile":           "",
		"archive.tar.gz":     "",
	}

	for filename, want := range tests {
		assert.Equal(t, Detect(filename), want)
	}
}

func TestName(t *testing.T) {
	assert.Equal(t, Name("go"), "Go")
	assert.Equal(t, Name(""), "Plain text")
	assert.Equal(t, Name("klingon"), "Plain text")
	assert.Equal(t, len(Values()), len(All))
}
