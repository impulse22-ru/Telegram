package indexer

import (
	"reflect"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		in       string
		cmd, arg string
	}{
		{"!search кот", "!search", "кот"},
		{"/start@MyBot", "/start", ""},
		{"!ban 12", "!ban", "12"},
		{"!stat", "!stat", ""},
		{"   !top   ", "!top", ""},
	}
	for _, c := range cases {
		cmd, arg := splitCommand(c.in)
		if cmd != c.cmd || arg != c.arg {
			t.Errorf("splitCommand(%q) = (%q,%q), want (%q,%q)", c.in, cmd, arg, c.cmd, c.arg)
		}
	}
}

func TestCaptionToTitle(t *testing.T) {
	if got := captionToTitle("Первая строка\nвторая"); got != "Первая строка" {
		t.Errorf("captionToTitle = %q", got)
	}
	if got := captionToTitle("только одна"); got != "только одна" {
		t.Errorf("captionToTitle = %q", got)
	}
	if got := captionToTitle("   \n "); got != "" {
		t.Errorf("captionToTitle whitespace = %q", got)
	}
}

func TestExtractTags(t *testing.T) {
	got := extractTags("Красиво #природа и еще #лето #sun")
	want := []string{"природа", "лето", "sun"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractTags = %v, want %v", got, want)
	}
}

func TestParseID(t *testing.T) {
	if parseID(" 42 ") != 42 {
		t.Error("parseID 42")
	}
	if parseID("abc") != 0 {
		t.Error("parseID abc")
	}
}