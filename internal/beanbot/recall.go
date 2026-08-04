package beanbot

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// namedUser is a Discord user the Trigger already identified — somebody who
// spoke in the Backlog, or somebody @-mentioned in the message — paired with
// the name they appear under.
//
// Pairing the two is what lets a Person survive a rename: the notes are headed
// with the old name, the conversation shows the new one, and only the snowflake
// spans both. It is also the only source of a snowflake BeanBot will trust,
// since Discord stated it rather than the model guessing it.
type namedUser struct {
	ID   string
	Name string
}

// commonWords are words too ordinary to decide anybody is In Play. Somebody
// called "Al" is fine — a whole-word match still finds them — but a Person
// aliased "the guy" would otherwise be in every conversation in the server.
var commonWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true,
	"is": true, "was": true, "it": true, "he": true, "she": true, "they": true,
	"him": true, "her": true, "them": true, "his": true, "that": true,
	"this": true, "who": true, "one": true, "all": true, "my": true,
	"guy": true, "guys": true, "man": true, "dude": true, "mate": true,
	"bloke": true, "kid": true, "boy": true, "girl": true, "lad": true,
	"new": true, "old": true, "some": true, "with": true, "about": true,
}

// recall assembles the Memory as it reaches one Trigger.
//
// Topical notes go in whole: they are prose, they are capped by Compaction, and
// nothing in a Backlog says which of them matters. The Roster cannot, because it
// grows with every human the server has ever discussed. So People arrive only
// when they are In Play, newest mention first, until the budget runs out —
// and everybody else arrives as a bare name, which costs about twenty bytes and
// is the difference between BeanBot saying "you mean Steve Steveson?" and
// denying that he has ever heard of him.
func recall(doc *memoryDoc, backlog string, speakers []namedUser, budget int) string {
	var b strings.Builder
	b.WriteString(doc.topical())

	present, absent := doc.roster.inPlay(backlog, speakers)

	var block strings.Builder
	// Orphans belong to nobody, so nothing can decide they are In Play. There
	// are few of them and they are small, so they simply always go in.
	for _, orphan := range doc.roster.orphans {
		block.WriteString("- " + orphan + "\n")
	}

	for _, in := range present {
		// Measured against what the block has already cost, separators and
		// orphans included, so the budget bounds what is actually sent rather
		// than the sum of the entries in it.
		entry, ok := in.who.recallEntry(budget - block.Len() - 1)
		if !ok {
			// No room for even their newest note. A heading with nothing under
			// it costs more than the name does and says less.
			absent = append(absent, in.who)
			continue
		}
		if block.Len() > 0 {
			block.WriteString("\n")
		}
		block.WriteString(entry)
	}

	if block.Len() > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + rosterHeading + "\n")
		b.WriteString(block.String())
	}

	if len(absent) > 0 {
		names := make([]string, 0, len(absent))
		for _, p := range absent {
			names = append(names, p.name)
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		// Labelled as names he knows without the details to hand. Get this wrong
		// and BeanBot denies knowing Steve while Steve's name sits in his prompt.
		b.WriteString(fmt.Sprintf(
			"Other people you have notes about, not in front of you right now: %s. "+
				"Say the name and the notes come back.\n",
			strings.Join(names, ", ")))
	}

	return b.String()
}

// inPlayPerson pairs a Person with how far into the Backlog they were last
// touched, which is the order the budget is spent in.
type inPlayPerson struct {
	who *person
	at  int
}

// inPlay splits the Roster into the People this conversation touches and the
// rest. A Person is In Play if a distinctive part of one of their names appears
// in the Backlog, or if they spoke in it.
func (r *roster) inPlay(backlog string, speakers []namedUser) (present []inPlayPerson, absent []*person) {
	haystack := strings.ToLower(backlog)

	for _, p := range r.people {
		at, spoke := -1, false

		for _, s := range speakers {
			if s.ID == "" || s.ID != p.id {
				continue
			}
			// Somebody in the room is In Play whether or not the match lands:
			// the Backlog prints their name, but it may be shaped oddly.
			spoke = true
			at = max(at, lastWholeWord(haystack, strings.ToLower(s.Name)))
		}
		for _, token := range p.tokens() {
			at = max(at, lastWholeWord(haystack, token))
		}

		if at >= 0 || spoke {
			present = append(present, inPlayPerson{who: p, at: max(at, 0)})
			continue
		}
		absent = append(absent, p)
	}

	// Whoever came up last is loaded first, so a budget that runs out cuts the
	// people who drifted out of the conversation rather than the current one.
	sort.SliceStable(present, func(i, j int) bool { return present[i].at > present[j].at })
	return present, absent
}

// tokens are the words that finding this Person in a conversation may turn on.
// Entries are headed with the fullest name known and channels use first names,
// so matching only the whole heading would mean owning a Steve nobody can find.
//
// The ordinary-word filter applies only *within* a name of several words, where
// dropping a word still leaves something to match on: "the Facebook guy" turns
// on "facebook". Somebody whose whole name is Guy keeps it, because suppressing
// it would make them permanently unfindable rather than merely less noisy.
func (p *person) tokens() []string {
	var out []string
	for _, term := range p.terms() {
		words := strings.Fields(strings.ToLower(term))
		for _, token := range words {
			token = strings.Trim(token, ".,;:!?'\"()[]")
			if len(token) < 2 || (len(words) > 1 && commonWords[token]) {
				continue
			}
			out = append(out, token)
		}
	}
	return out
}

// recallEntry renders a Person for the prompt within the space left, reporting
// false when not even their newest note fits.
//
// Truncation is a reading decision, not a forgetting one: the file on disk still
// holds every note. Newest first, because recent material is the more
// representative of how somebody is now — and the cut is announced, or BeanBot
// will talk as though the older notes were never written.
func (p *person) recallEntry(budget int) (string, bool) {
	head := "### " + p.name
	if len(p.aliases) > 0 {
		// The snowflake is deliberately not here. It is how Go finds a Person
		// across a rename; the model has no use for it.
		head += " (also known as " + strings.Join(p.aliases, ", ") + ")"
	}
	head += "\n"

	var full strings.Builder
	full.WriteString(head)
	for _, fact := range p.facts {
		full.WriteString("- " + fact + "\n")
	}
	if full.Len() <= budget {
		return full.String(), true
	}

	marker := fmt.Sprintf("- …older notes about %s are on file, not shown here.\n", p.name)
	size := len(head) + len(marker)

	var kept []string
	for i := len(p.facts) - 1; i >= 0; i-- {
		line := "- " + p.facts[i] + "\n"
		if size+len(line) > budget {
			break
		}
		size += len(line)
		kept = append([]string{line}, kept...)
	}
	if len(kept) == 0 {
		return "", false
	}

	return head + marker + strings.Join(kept, ""), true
}

// lastWholeWord finds where needle last appears in haystack as a word rather
// than as part of one, so "Al" does not match "all". Both must already be
// lowercase. It returns -1 when there is no such match.
func lastWholeWord(haystack, needle string) int {
	if needle == "" {
		return -1
	}

	for from := len(haystack); from >= 0; {
		i := strings.LastIndex(haystack[:from], needle)
		if i < 0 {
			return -1
		}
		if !wordCharBefore(haystack, i) && !wordCharAt(haystack, i+len(needle)) {
			return i
		}
		from = i
	}
	return -1
}

func wordCharBefore(s string, i int) bool {
	r, size := utf8.DecodeLastRuneInString(s[:i])
	return size > 0 && isWordChar(r)
}

func wordCharAt(s string, i int) bool {
	r, size := utf8.DecodeRuneInString(s[i:])
	return size > 0 && isWordChar(r)
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
