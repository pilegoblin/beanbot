package beanbot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestShortReplyIsASingleMessage(t *testing.T) {
	// The old behaviour split on every newline, turning one reply into a
	// flood of notifications.
	got := splitMessage("line one\nline two\nline three", 2000)

	if len(got) != 1 {
		t.Errorf("expected one message, got %d: %q", len(got), got)
	}
}

func TestNoChunkExceedsTheLimit(t *testing.T) {
	long := strings.Repeat("word ", 1200) // 6000 chars

	for i, chunk := range splitMessage(long, 2000) {
		if len(chunk) > 2000 {
			t.Errorf("chunk %d is %d chars, over the 2000 limit", i, len(chunk))
		}
	}
}

func TestSplitsPreferParagraphBoundaries(t *testing.T) {
	a := strings.Repeat("a", 30)
	b := strings.Repeat("b", 30)

	got := splitMessage(a+"\n\n"+b, 40)

	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("expected a clean split at the blank line, got %q", got)
	}
}

func TestSplitsNeverLandMidWord(t *testing.T) {
	// "supercalifragilistic supercalifragilistic ..." with a limit that forces
	// a split partway through the run of words.
	text := strings.TrimSpace(strings.Repeat("supercalifragilistic ", 10))

	for _, chunk := range splitMessage(text, 100) {
		for _, word := range strings.Fields(chunk) {
			if word != "supercalifragilistic" {
				t.Errorf("a word was cut in half: %q", word)
			}
		}
	}
}

func TestUnbreakableRunIsHardSplit(t *testing.T) {
	// A 5000-character URL has no boundary to split on, but Discord will still
	// reject it whole, so it has to be cut somewhere.
	got := splitMessage(strings.Repeat("x", 5000), 2000)

	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	if total := len(strings.Join(got, "")); total != 5000 {
		t.Errorf("hard split lost characters: %d of 5000 survived", total)
	}
}

func TestHardSplitNeverCutsARuneInHalf(t *testing.T) {
	// Every character here is 4 bytes with no spaces to break on, so a
	// byte-indexed cut lands mid-rune and emits invalid UTF-8.
	text := strings.Repeat("🫘", 50)

	for i, chunk := range splitMessage(text, 20) {
		if !utf8.ValidString(chunk) {
			t.Errorf("chunk %d is not valid UTF-8: %q", i, chunk)
		}
	}
}

func TestLimitCountsCharactersNotBytes(t *testing.T) {
	// 30 four-byte runes is 120 bytes but only 30 characters, so it fits in a
	// 50-character message and must not be split at all.
	text := strings.Repeat("🫘", 30)

	if got := splitMessage(text, 50); len(got) != 1 {
		t.Errorf("expected one message, got %d", len(got))
	}
}

func TestEmptyReplyProducesNothingToSend(t *testing.T) {
	if got := splitMessage("   \n\n  ", 2000); len(got) != 0 {
		t.Errorf("expected nothing to send, got %q", got)
	}
}
