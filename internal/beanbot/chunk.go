package beanbot

import (
	"strings"
	"unicode/utf8"
)

// discordMessageLimit is the maximum length of a single Discord message.
const discordMessageLimit = 2000

// splitMessage breaks a reply into as few Discord messages as possible.
// A reply that fits is sent whole — splitting on every newline, as BeanBot
// once did, turns a single answer into a flood of notifications.
//
// When a split is unavoidable it lands on the most natural boundary
// available: a blank line, then a line break, then a space. Text with no
// boundary at all (a very long URL) is cut at the limit, since Discord
// rejects it either way. Limits count characters, not bytes, so a cut never
// lands inside a multi-byte rune.
func splitMessage(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var chunks []string
	for utf8.RuneCountInString(text) > limit {
		split := boundaryBefore(text, limit)
		chunks = append(chunks, strings.TrimSpace(text[:split]))
		text = strings.TrimSpace(text[split:])
	}

	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

// boundaryBefore returns the byte index of the latest natural break within the
// first limit characters, falling back to the character boundary at the limit
// when the text offers nothing to break on.
func boundaryBefore(text string, limit int) int {
	window := text[:byteIndexOfRune(text, limit)]
	for _, sep := range []string{"\n\n", "\n", " "} {
		if i := strings.LastIndex(window, sep); i > 0 {
			return i
		}
	}
	return len(window)
}

// byteIndexOfRune returns the byte offset at which the nth rune starts, or the
// length of the string if it has fewer than n runes.
func byteIndexOfRune(text string, n int) int {
	count := 0
	for i := range text {
		if count == n {
			return i
		}
		count++
	}
	return len(text)
}
