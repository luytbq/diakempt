package unistr

import "testing"

func TestWhitespaceIncludesSeparatorChars(t *testing.T) {
	for r := rune(0x1c); r <= 0x1f; r++ {
		if !IsSpace(r) {
			t.Errorf("U+%04X must be whitespace", r)
		}
	}
	if Strip("\x1f a \x1c") != "a" {
		t.Errorf("Strip does not trim U+001C and U+001F")
	}
}

func TestLowerDottedCapitalIGivesTwoChars(t *testing.T) {
	if got := Lower("İD"); got != "i̇d" {
		t.Errorf("Lower(İD) = %q, want %q", got, "i̇d")
	}
}
