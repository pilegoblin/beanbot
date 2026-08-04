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
// write would retype every fact through the model on every call, turning
// paraphrase drift from an occasional cost of Compaction into a certainty of
// every write — and it would overwrite from a snapshot read seconds earlier,
// silently erasing anything recorded in between.
type change struct {
	// Section is the heading to file the entry under, created if absent.
	Section string
	// Entry is the line to record, already carrying its attribution.
	Entry string
	// Replaces quotes an existing entry this one supersedes. Empty adds.
	Replaces string
}

// memoryDoc is a parsed Memory: headings, and the entries beneath them.
type memoryDoc struct {
	// preamble is whatever sits above the first heading. Nothing BeanBot writes
	// puts text there, but Compaction is a language model and might, so it
	// survives a round trip rather than being silently dropped.
	preamble []string
	sections []*memorySection
}

type memorySection struct {
	title   string
	entries []string
}

// applyChange records one change against a rendered Memory and returns the new
// document.
func applyChange(raw string, ch change) (string, error) {
	doc := parseMemory(raw)
	if err := doc.apply(ch); err != nil {
		return "", err
	}
	return doc.render(), nil
}

func (d *memoryDoc) apply(ch change) error {
	title := strings.TrimSpace(ch.Section)
	entry := strings.TrimSpace(ch.Entry)
	if title == "" {
		return errors.New("a memory needs a section to file it under")
	}
	if entry == "" {
		return errors.New("a memory needs something to record")
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

// find locates the entry a change supersedes. The match is loose — normalised
// containment — because entries carry a trailing attribution the model will not
// reproduce exactly, and an exact-match rule would make correction unusable.
func (d *memoryDoc) find(needle string) (*memorySection, int) {
	want := normaliseEntry(needle)
	if want == "" {
		return nil, 0
	}

	for _, s := range d.sections {
		for i, entry := range s.entries {
			if strings.Contains(normaliseEntry(entry), want) {
				return s, i
			}
		}
	}
	return nil, 0
}

func normaliseEntry(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func parseMemory(raw string) *memoryDoc {
	doc := &memoryDoc{}
	var current *memorySection

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			continue

		case strings.HasPrefix(trimmed, "#"):
			current = &memorySection{title: strings.TrimSpace(strings.TrimLeft(trimmed, "#"))}
			doc.sections = append(doc.sections, current)

		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			entry := strings.TrimSpace(trimmed[2:])
			if current == nil {
				doc.preamble = append(doc.preamble, trimmed)
				continue
			}
			current.entries = append(current.entries, entry)

		default:
			// Neither heading nor bullet. Compaction writes this file, so stray
			// prose is possible; it is folded into the entry above rather than
			// dropped, because losing a recorded fact is the worse failure.
			switch {
			case current == nil:
				doc.preamble = append(doc.preamble, trimmed)
			case len(current.entries) > 0:
				current.entries[len(current.entries)-1] += " " + trimmed
			default:
				current.entries = append(current.entries, trimmed)
			}
		}
	}

	return doc
}

func (d *memoryDoc) render() string {
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
