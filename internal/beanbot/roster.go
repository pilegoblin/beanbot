package beanbot

import (
	"errors"
	"fmt"
	"strings"
)

// rosterHeading is the heading the Roster lives under. It is reserved: topical
// notes may not be filed there, because a Person is addressed by name and a
// topical entry is not.
const rosterHeading = "People"

// person is one Person in a Guild's Roster — a human BeanBot has learned
// something about, whether or not they are in the Guild.
//
// name is what BeanBot calls them and heads their entry; id is the Discord
// snowflake, present only when Discord stated it directly, and empty for
// everyone who was merely talked about. Display names are the most mutable
// handle Discord offers, so the id is what survives a rename.
type person struct {
	name    string
	id      string
	aliases []string
	facts   []string
}

// roster is every Person in one Guild, plus any bullets found directly under
// the People heading. Those orphans belong to nobody — Compaction writes
// freely-shaped markdown and the file can be hand-edited — and are kept
// verbatim rather than guessed at, because losing a recorded fact is worse
// than keeping an oddly placed one.
type roster struct {
	orphans []string
	people  []*person
}

// personChange is one edit to the Roster: a fact about somebody, optionally
// superseding a fact already recorded about them.
//
// ID is filled in by Go, never by the model. A wrong snowflake welds two humans
// together permanently and nothing downstream will ever question it, so it is
// captured only where Discord stated it directly.
type personChange struct {
	Name     string
	Fact     string
	Aliases  []string
	ID       string
	Replaces string
}

func applyPersonChange(raw string, ch personChange) (string, error) {
	return editMemory(raw, func(d *memoryDoc) error { return d.roster.apply(ch) })
}

func applyMerge(raw, from, into string) (string, error) {
	return editMemory(raw, func(d *memoryDoc) error { return d.roster.merge(from, into) })
}

func (r *roster) apply(ch personChange) error {
	name := strings.TrimSpace(ch.Name)
	fact := cleanFact(ch.Fact)
	if name == "" {
		return errors.New("a note about somebody needs their name")
	}
	if fact == "" {
		return errors.New("a note about somebody needs something to record")
	}

	p := r.find(name, ch.ID)
	if p == nil {
		p = &person{name: name}
		r.people = append(r.people, p)
	}

	// Only ever fills a gap. Replacing an identity already on file is the one
	// mistake that cannot be spotted afterwards, so the first one recorded wins.
	if p.id == "" {
		p.id = ch.ID
	}

	// A Person found under a name that is not their heading — an alias, or the
	// name they now go by after a rename — keeps that name as a way of finding
	// them again.
	p.learnAliases(append([]string{name}, ch.Aliases...))

	replaces := strings.TrimSpace(ch.Replaces)
	if replaces == "" {
		p.facts = append(p.facts, fact)
		return nil
	}

	i := findEntry(p.facts, replaces)
	if i < 0 {
		// Reported back to the model, which can retry as a plain addition rather
		// than silently recording a correction to nothing.
		return fmt.Errorf("nothing recorded about %s matches %q — quote an existing note to "+
			"correct it, or leave that out to record something new", p.name, replaces)
	}
	p.facts[i] = fact
	return nil
}

// merge folds one Person into another once both turn out to be the same human.
// Every fact moves across and the absorbed name survives as an alias, so a
// merge loses nothing — which is why, unlike a deletion, it needs no Gate.
func (r *roster) merge(from, into string) error {
	loser := r.find(strings.TrimSpace(from), "")
	if loser == nil {
		return fmt.Errorf("i have no notes about %q", from)
	}
	winner := r.find(strings.TrimSpace(into), "")
	if winner == nil {
		return fmt.Errorf("i have no notes about %q", into)
	}
	if loser == winner {
		// Silently succeeding would report a duplicate fixed that is still there.
		return fmt.Errorf("%q and %q are already the same person in my notes", from, into)
	}

	winner.facts = append(winner.facts, loser.facts...)
	winner.learnAliases(append([]string{loser.name}, loser.aliases...))
	if winner.id == "" {
		winner.id = loser.id
	}

	for i, p := range r.people {
		if p == loser {
			r.people = append(r.people[:i], r.people[i+1:]...)
			break
		}
	}
	return nil
}

// render writes the Roster back out. precededByText spaces it off from whatever
// the topical notes left above it.
func (r *roster) render(precededByText bool) string {
	if len(r.people) == 0 && len(r.orphans) == 0 {
		return ""
	}

	var b strings.Builder
	if precededByText {
		b.WriteString("\n")
	}
	b.WriteString("## " + rosterHeading + "\n")

	for _, orphan := range r.orphans {
		b.WriteString("- " + orphan + "\n")
	}

	for _, p := range r.people {
		// Only somebody with nothing recorded about them at all — no facts, no
		// identity, nothing but a heading — is dropped. A Person carrying only
		// an identity line still knows who they are, and that is a recorded fact
		// like any other.
		if len(p.facts) == 0 && p.identityLine() == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(p.render())
	}

	return b.String()
}

// find locates a Person by snowflake first, then by their heading, then by an
// alias. The snowflake wins because it is the only handle a rename cannot move.
func (r *roster) find(name, id string) *person {
	if id != "" {
		for _, p := range r.people {
			if p.id == id {
				return p
			}
		}
	}
	if name == "" {
		return nil
	}

	for _, p := range r.people {
		if strings.EqualFold(p.name, name) {
			return p
		}
	}
	for _, p := range r.people {
		if containsFold(p.aliases, name) {
			return p
		}
	}
	return nil
}

// names lists every Person's heading, which is what the Name Index carries.
func (r *roster) names() []string {
	out := make([]string, 0, len(r.people))
	for _, p := range r.people {
		out = append(out, p.name)
	}
	return out
}

// learnAliases records other names this Person answers to, ignoring anything
// that only repeats what is already on file.
func (p *person) learnAliases(candidates []string) {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" || strings.EqualFold(c, p.name) || containsFold(p.aliases, c) {
			continue
		}
		p.aliases = append(p.aliases, c)
	}
}

func containsFold(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.EqualFold(s, needle) {
			return true
		}
	}
	return false
}

// terms is every name this Person might be called in a conversation, which is
// what decides whether they are In Play.
func (p *person) terms() []string {
	return append([]string{p.name}, p.aliases...)
}

func (p *person) render() string {
	var b strings.Builder
	b.WriteString("### " + p.name + "\n")
	if line := p.identityLine(); line != "" {
		b.WriteString(line + "\n")
	}
	for _, fact := range p.facts {
		b.WriteString("- " + fact + "\n")
	}
	return b.String()
}

// identityLine renders the snowflake and aliases as an HTML comment: a shape
// that cannot collide with a fact, since facts are member-authored text and one
// that parsed as an identity would let anyone staple a snowflake onto anyone.
// It stays plainly readable in the file, which is the point of a markdown
// Memory readable over `fly ssh`.
func (p *person) identityLine() string {
	var parts []string
	if p.id != "" {
		parts = append(parts, "id: "+p.id)
	}
	if len(p.aliases) > 0 {
		parts = append(parts, "aka: "+strings.Join(p.aliases, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "<!-- " + strings.Join(parts, "; ") + " -->"
}

func isIdentityLine(line string) bool {
	return strings.HasPrefix(line, "<!--") && strings.HasSuffix(line, "-->")
}

func parseIdentityLine(line string) (id string, aliases []string) {
	body := strings.TrimSuffix(strings.TrimPrefix(line, "<!--"), "-->")

	for _, field := range strings.Split(body, ";") {
		key, value, ok := strings.Cut(field, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)

		switch strings.ToLower(strings.TrimSpace(key)) {
		case "id":
			id = value
		case "aka":
			for _, alias := range strings.Split(value, ",") {
				if alias = strings.TrimSpace(alias); alias != "" {
					aliases = append(aliases, alias)
				}
			}
		}
	}
	return id, aliases
}

// cleanFact flattens a fact onto one line and strips any HTML comment from it,
// so member-authored text cannot forge an identity line or a heading and end up
// rewriting who somebody is.
func cleanFact(fact string) string {
	for {
		open := strings.Index(fact, "<!--")
		if open < 0 {
			break
		}
		end := strings.Index(fact[open:], "-->")
		if end < 0 {
			fact = fact[:open]
			break
		}
		fact = fact[:open] + fact[open+end+len("-->"):]
	}

	return strings.Join(strings.Fields(fact), " ")
}

// findEntry locates the entry a change supersedes. The match is loose —
// normalised containment — because entries carry a trailing attribution the
// model will not reproduce exactly, and an exact-match rule would make
// correction unusable.
func findEntry(entries []string, needle string) int {
	want := normaliseEntry(needle)
	if want == "" {
		return -1
	}

	for i, entry := range entries {
		if strings.Contains(normaliseEntry(entry), want) {
			return i
		}
	}
	return -1
}
