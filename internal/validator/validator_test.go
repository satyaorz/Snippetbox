package validator

import (
	"testing"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestValidators(t *testing.T) {
	assert.Equal(t, NotBlank("  "), false)
	assert.Equal(t, NotBlank(" a "), true)
	assert.Equal(t, MaxChars("héllo", 5), true)
	assert.Equal(t, MaxChars("héllo!", 5), false)
	assert.Equal(t, MinChars("pa$$word", 8), true)
	assert.Equal(t, MinChars("pa$$", 8), false)
	assert.Equal(t, PermittedValue(7, 1, 7, 365), true)
	assert.Equal(t, PermittedValue(42, 1, 7, 365), false)
	assert.Equal(t, Matches("bob@example.com", EmailRX), true)
	assert.Equal(t, Matches("bob@example.", EmailRX), false)
}

func TestValidatorErrors(t *testing.T) {
	var v Validator
	assert.Equal(t, v.Valid(), true)

	v.CheckField(false, "title", "first")
	v.CheckField(false, "title", "second")
	assert.Equal(t, v.FieldErrors["title"], "first")
	assert.Equal(t, v.Valid(), false)

	var v2 Validator
	v2.AddNonFieldError("bad credentials")
	assert.Equal(t, v2.Valid(), false)
}
