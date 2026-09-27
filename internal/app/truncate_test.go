package app

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateKeepsValidUTF8(t *testing.T) {
	s := strings.Repeat("é", 10) // 2 bytes each
	got := truncate(s, 5)        // cuts the third character in half
	if !utf8.ValidString(got) || !strings.HasPrefix(got, "éé\n[truncated 15 bytes]") {
		t.Fatalf("got %q", got)
	}
	if truncate("short", 10) != "short" {
		t.Fatal("short strings pass through")
	}
}
