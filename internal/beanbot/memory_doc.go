package beanbot

import (
	"errors"
	"fmt"
	"strings"
)

// change is one edit to a Guild's Memory: an entry to file under a section,
// optionally superseding something already recorded.
//
// The model submits a change rather than a rewritten document. A whole-document
// write would retype every Claim through the model on every call, turning
// paraphrase drift from an occasional cost of Compaction into a certainty of
// every write — and it would overwrite from a snapshot read seconds earlier,
// silently erasing anything recorded in between.
type change struct {
	// Section is the heading to file the Claim under, created if absent.
	Section string
	// Claim is the line to record, already carrying its Attribution.
	Claim string
	// Replaces quotes an existing entry this one supersedes. Empty adds.
	Replaces string
}

// memoryDoc is a parsed Memory: topical headings with entries beneath them,
// and the Roster of People.
//
// The two are kept apart because they obey different rules. Topical notes are
// prose: compressible, and injected whole. The Roster is a list of humans:
// never compacted, never discarded, and read back only for the People a
// conversation actually touches.
type memoryDoc struct {
	// preamble is whatever sits above the first heading. Nothing BeanBot writes
	// puts text there, but Compaction is a language model and might, so it
	// survives a round trip rather than being silently dropped.
	preamble []string
	sections []*memorySection
	roster   *roster
}

type memorySection struct {
	title   string
	entries []string
}

// applyChange records one change against a rendered Memory and returns the new
// document.
func applyChange(raw string, ch change) (string, error) {
	return editMemory(raw, func(d *memoryDoc) error { return d.apply(ch) })
}

// editMemory parses a Memory, edits it, and renders it back. Every write goes
// through here, so a change is always merged into the document as it is now
// rather than replacing it wholesale.
func editMemory(raw string, edit func(*memoryDoc) error) (string, error) {
	doc := parseMemory(raw)
	if err := edit(doc); err != nil {
		return "", err
	}
	return doc.render(), nil
}

func (d *memoryDoc) apply(ch change) error {
	title := strings.TrimSpace(ch.Section)
	entry := strings.TrimSpace(ch.Claim)
	if title == "" {
		return errors.New("a memory needs a section to file it under")
	}
	if entry == "" {
		return errors.New("a memory needs something to record")
	}
	// Reported back so the model retries through the right tool. Letting a
	// topical entry land in the Roster would split what BeanBot knows about
	// somebody between a heading with their name on it and a loose bullet.
	if strings.EqualFold(title, rosterHeading) {
		return errors.New("notes about a person go through remember_person, not here — " +
			"this is for topics like traditions and running jokes")
	}

	target := d.section(title)

	replaces := strings.TrimSpace(ch.Replaces)
	if replaces == "" {
		target.entries = append(target.entries, entry)
		return nil
	}

	from, i := d.find(replaces)
	if from == nil {
		// Reported back to the model, which can retry as a plain addition
		// rather than silently recording a correction to nothing.
		return fmt.Errorf("nothing recorded matches %q — quote an existing line to replace it, "+
			"or leave that out to record something new", replaces)
	}

	// Replacing within the same section keeps its position, so an ordinary
	// correction does not reshuffle the document. Across sections it is a
	// re-filing: remove it there, add it here.
	if from == target {
		target.entries[i] = entry
		return nil
	}

	from.entries = append(from.entries[:i], from.entries[i+1:]...)
	target.entries = append(target.entries, entry)
	return nil
}

// section finds a heading or creates it. Matching ignores case because the
// model retypes the heading from what it was shown, and a capitalisation
// difference would split one topic across two sections.
func (d *memoryDoc) section(title string) *memorySection {
	for _, s := range d.sections {
		if strings.EqualFold(s.title, title) {
			return s
		}
	}

	s := &memorySection{title: title}
	d.sections = append(d.sections, s)
	return s
}

// find locates the entry a change supersedes, anywhere in the topical notes.
func (d *memoryDoc) find(needle string) (*memorySection, int) {
	for _, s := range d.sections {
		if i := findEntry(s.entries, needle); i >= 0 {
			return s, i
		}
	}
	return nil, 0
}

func normaliseEntry(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func parseMemory(raw string) *memoryDoc {
	doc := &memoryDoc{roster: &roster{}}
	var current *memorySection
	var who *person
	inRoster := false

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			continue

		case strings.HasPrefix(trimmed, "#"):
			title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))

			// Inside the Roster a deeper heading names a Person. Anything else
			// closes the region, so a topical section written after it is not
			// swallowed by the People it follows.
			if inRoster && headingLevel(trimmed) >= 3 {
				who = &person{name: title}
				doc.roster.people = append(doc.roster.people, who)
				continue
			}

			current, who = nil, nil
			if inRoster = strings.EqualFold(title, rosterHeading); inRoster {
				continue
			}
			current = &memorySection{title: title}
			doc.sections = append(doc.sections, current)

		// Only ever the line directly under a Person's heading. Anywhere else it
		// is text somebody wrote, not a claim about who they are.
		case who != nil && len(who.claims) == 0 && isIdentityLine(trimmed):
			who.id, who.aliases = parseIdentityLine(trimmed)

		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			entry := strings.TrimSpace(trimmed[2:])
			switch {
			case who != nil:
				who.claims = append(who.claims, entry)
			case inRoster:
				doc.roster.orphans = append(doc.roster.orphans, entry)
			case current == nil:
				doc.preamble = append(doc.preamble, trimmed)
			default:
				current.entries = append(current.entries, entry)
			}

		default:
			// Neither heading nor bullet. Compaction writes this file, so stray
			// prose is possible; it is folded into the entry above rather than
			// dropped, because losing a recorded fact is the worse failure.
			switch {
			case who != nil:
				who.claims = appendProse(who.claims, trimmed)
			case inRoster:
				doc.roster.orphans = appendProse(doc.roster.orphans, trimmed)
			case current == nil:
				doc.preamble = append(doc.preamble, trimmed)
			default:
				current.entries = appendProse(current.entries, trimmed)
			}
		}
	}

	return doc
}

// headingLevel counts the hashes, which is what separates a Person from a
// section once the Roster has been entered.
func headingLevel(line string) int {
	return len(line) - len(strings.TrimLeft(line, "#"))
}

// appendProse folds a continuation line into the entry it follows, or keeps it
// as an entry of its own when there is nothing to attach it to.
func appendProse(entries []string, line string) []string {
	if len(entries) == 0 {
		return append(entries, line)
	}
	entries[len(entries)-1] += " " + line
	return entries
}

func (d *memoryDoc) render() string {
	topical := d.topical()

	// The Roster is rendered last, whatever order it was read in: it is the
	// region that grows without limit, so keeping it below the topical notes
	// leaves the compactible part of the file where a human can still find it.
	return topical + d.roster.render(topical != "")
}

// topical renders everything that is not the Roster. It is what Compaction is
// given, and what goes into every Trigger whole.
func (d *memoryDoc) topical() string {
	var b strings.Builder

	for _, line := range d.preamble {
		b.WriteString(line)
		b.WriteString("\n")
	}

	for _, s := range d.sections {
		// A section emptied by re-filing its last entry is not worth a heading.
		if len(s.entries) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}

		b.WriteString("## " + s.title + "\n")
		for _, entry := range s.entries {
			b.WriteString("- " + entry + "\n")
		}
	}

	return b.String()
}
