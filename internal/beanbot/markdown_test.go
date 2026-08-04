package beanbot

import (
	"strings"
	"testing"
)

func TestUnderscoreEmoticonsSurviveDiscordFormatting(t *testing.T) {
	// "o_O ^_^" pairs its underscores and italicises everything between them,
	// so Discord shows "o" then a slanted "O ^" then "^".
	got := escapeMarkdown("o_O ^_^")

	if got != `o\_O ^\_^` {
		t.Errorf("got %q, want %q", got, `o\_O ^\_^`)
	}
}

func TestStarEyedEmoticonIsNotTurnedIntoItalics(t *testing.T) {
	if got := escapeMarkdown("*_*"); got != `\*\_\*` {
		t.Errorf("got %q", got)
	}
}

func TestShruggingEmoticonKeepsItsBackslash(t *testing.T) {
	// The backslash in ¯\_(ツ)_/¯ is itself markup to Discord; escaping the
	// underscores without escaping the backslash eats the arm.
	got := escapeMarkdown(`¯\_(ツ)_/¯`)

	if !strings.HasPrefix(got, `¯\\`) {
		t.Errorf("backslash was not escaped: %q", got)
	}
}

func TestAngryEmoticonAtLineStartIsNotAQuote(t *testing.T) {
	// A leading ">" makes the whole line a blockquote.
	got := escapeMarkdown(">_< that hurt")

	if !strings.HasPrefix(got, `\>`) {
		t.Errorf("leading > was not escaped: %q", got)
	}
}

func TestSpoilerBarsAreNeutralised(t *testing.T) {
	if got := escapeMarkdown("||hidden||"); strings.Contains(got, "||") {
		t.Errorf("spoiler markup survived: %q", got)
	}
}

func TestLinksAreLeftAlone(t *testing.T) {
	// Escaping inside a URL breaks Discord's auto-linking, and URLs with
	// underscores are common.
	text := "look at https://example.com/a_b_c cool"

	if got := escapeMarkdown(text); got != text {
		t.Errorf("the link was mangled: %q", got)
	}
}

func TestOrdinaryProseIsUnchanged(t *testing.T) {
	text := "That is the spectral manifestation of our own digital mediocrity."

	if got := escapeMarkdown(text); got != text {
		t.Errorf("prose should pass through untouched, got %q", got)
	}
}

func TestSpacingIsPreserved(t *testing.T) {
	if got := escapeMarkdown("a  b\nc"); got != "a  b\nc" {
		t.Errorf("spacing changed: %q", got)
	}
}

func TestAHardSplitNeverSeparatesABackslashFromWhatItEscapes(t *testing.T) {
	// Escaping happens before splitting, so a cut landing between "\" and "_"
	// would emit a stray backslash and un-escape the next chunk's first char.
	escaped := escapeMarkdown(strings.Repeat("o_O", 400))

	for _, chunk := range splitMessage(escaped, 50) {
		if strings.HasSuffix(chunk, `\`) {
			t.Fatalf("chunk ends with a dangling escape: %q", chunk)
		}
	}
}
