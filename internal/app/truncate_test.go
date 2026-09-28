package app

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateKeepsValidUTF8(t *testing.T) {
	s := strings.Repeat("\u00e9", 10) // two bytes each in UTF-8
	got := truncate(s, 5)             // cuts the third character in half
	if !utf8.ValidString(got) || !strings.HasPrefix(got, "\u00e9\u00e9\n[truncated 15 bytes]") {
		t.Fatalf("got %q", got)
	}
	if truncate("short", 10) != "short" {
		t.Fatal("short strings pass through")
	}
}
