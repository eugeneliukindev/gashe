package memory_test

import (
	"strings"
	"testing"

	"github.com/eugeneliukindev/gashe/backend/memory"
)

// Any pattern and key: no panic, and a pattern that escapes every byte of a key matches it.
func FuzzMatch(f *testing.F) {
	for _, s := range []string{"user:*", "a?c", "[a-c]x", "[^ab]", `\*`, "[", "*[", "a-]"} {
		f.Add(s, "user:42")
	}
	f.Fuzz(func(t *testing.T, pattern, key string) {
		if len(pattern) > 64 || len(key) > 64 { // stars backtrack: keep each run short
			return
		}
		_ = memory.Match(pattern, key)
		var esc strings.Builder
		for i := range len(key) {
			esc.WriteByte('\\')
			esc.WriteByte(key[i])
		}
		if !memory.Match(esc.String(), key) {
			t.Errorf("match(%q, %q) = false, want true", esc.String(), key)
		}
		if !memory.Match("*", key) {
			t.Errorf("match(\"*\", %q) = false, want true", key)
		}
	})
}
