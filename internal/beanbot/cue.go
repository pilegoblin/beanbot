package beanbot

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

// Cueing is what a Trigger offers Capabilities: its own words, and — when the
// message it replies to is one of BeanBot's own files — the Medium of that
// file. Someone who hits reply on a Clip is asking for another one however
// they word it, and that is a fact about the message rather than a guess at
// what it means.
type Cueing struct {
	Text    string
	ReplyTo Medium
}

// cueing reads a Trigger. Only BeanBot's own attachments count towards ReplyTo:
// a reply to somebody else's recording is a question about it, not a request
// for one.
func cueing(m *discordgo.Message, botID string) Cueing {
	cue := Cueing{Text: m.Content}

	ref := m.ReferencedMessage
	if ref == nil || ref.Author == nil || ref.Author.ID != botID {
		return cue
	}
	for _, a := range ref.Attachments {
		if medium := mediumOf(a); medium != NoMedium {
			cue.ReplyTo = medium
			break
		}
	}
	return cue
}

// onOffer reports whether a Capability is declared to the model for this
// Trigger. A Capability with no Cues is always On Offer, which is the ordinary
// case; one with Cues has to be asked for.
//
// Withholding rather than refusing is the whole point. ADR 0005 put the length
// cap in Go because Memory is member-writable and injected into every prompt,
// so a limit the model merely knows about is one it can be talked out of.
// "Speak whether or not they asked" is the same attack against a tool
// description, and a tool that was never declared has no description to argue
// with.
func onOffer(c Capability, cue Cueing) bool {
	cues := c.Cues()
	if len(cues) == 0 {
		return true
	}
	if medium := c.Medium(); medium != NoMedium && cue.ReplyTo == medium {
		return true
	}
	return cued(cue.Text, cues)
}

// cued reports whether the text contains any of the Cues as whole words.
//
// Whole words, because the Cues are short and common inside longer ones:
// substring matching would find "voice" in "invoice", "sing" in "using", and
// "audio" in a filename. Being On Offer is not being called — the tool
// description still decides — but a Cue that fires on "invoice" is a Cue that
// is not doing anything.
func cued(text string, cues []string) bool {
	lower := strings.ToLower(text)
	for _, cue := range cues {
		if containsWord(lower, cue) {
			return true
		}
	}
	return false
}

// containsWord finds word as a whole word in text, which is already lowercased.
// The word may itself contain a space — "out loud" is one Cue, not two.
func containsWord(text, word string) bool {
	for from := 0; from+len(word) <= len(text); {
		at := strings.Index(text[from:], word)
		if at < 0 {
			return false
		}
		start := from + at
		if standsAlone(text, start, start+len(word)) {
			return true
		}
		from = start + 1
	}
	return false
}

// standsAlone reports whether text[start:end] is bounded by something other
// than more word. Punctuation, spaces and the ends of the string all count, so
// "sing!" and "(aloud)" match while "singapore" does not.
func standsAlone(text string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if isWordRune(before) {
			return false
		}
	}
	if end < len(text) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if isWordRune(after) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
