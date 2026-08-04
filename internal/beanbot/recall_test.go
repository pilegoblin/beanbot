package beanbot

import (
	"strings"
	"testing"
)

// recalled runs recall over a parsed document with a generous budget.
func recalled(doc, backlog string, speakers ...namedUser) string {
	return recall(parseMemory(doc), backlog, speakers, 8<<10)
}

const twoPeople = "## People\n\n### Steve Steveson\n- Posts about model trains.\n\n" +
	"### Kate Kateson\n- Restores arcade cabinets.\n"

func TestTopicalNotesAlwaysReachThePrompt(t *testing.T) {
	// They are prose, they are capped, and there is no way to know from a
	// Backlog which of them matters. Only People are selected.
	doc := "## Traditions\n- Thursday is game night.\n" + twoPeople

	got := recalled(doc, "[12:00] drew: anyone about?")

	if !strings.Contains(got, "Thursday is game night.") {
		t.Errorf("the topical notes went missing: %q", got)
	}
}

func TestSomebodyNamedInTheBacklogArrivesInFull(t *testing.T) {
	got := recalled(twoPeople, "[12:00] drew: talk like Steve Steveson")

	if !strings.Contains(got, "Posts about model trains.") {
		t.Errorf("the person named was not recalled: %q", got)
	}
}

func TestSomebodyNobodyMentionedIsOnlyAName(t *testing.T) {
	// The whole point of selecting: a Roster of eighty people cannot ride on
	// every message, but BeanBot must still know that he knows them.
	got := recalled(twoPeople, "[12:00] drew: talk like Steve Steveson")

	if strings.Contains(got, "Restores arcade cabinets.") {
		t.Errorf("an unmentioned person was recalled in full: %q", got)
	}
	if !strings.Contains(got, "Kate Kateson") {
		t.Errorf("an unmentioned person vanished entirely: %q", got)
	}
}

func TestTheNameIndexSaysTheDetailsAreNotInFrontOfHim(t *testing.T) {
	// Mislabelled, BeanBot denies knowing Kate while her name sits in his own
	// prompt.
	got := recalled(twoPeople, "[12:00] drew: talk like Steve Steveson")

	var index string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "Kate Kateson") {
			index = line
		}
	}

	if index == "" {
		t.Fatalf("the unmentioned person is nowhere in the prompt: %q", got)
	}
	if !strings.Contains(index, "not in front of you") {
		t.Errorf("the index must say the details are merely absent, got %q", index)
	}
}

func TestAFirstNameFindsSomebodyRecordedInFull(t *testing.T) {
	// Entries are headed with the fullest name known; channels use first names.
	// Matching only the whole heading would mean owning a Steve nobody can find.
	got := recalled(twoPeople, "[12:00] drew: what does steve reckon")

	if !strings.Contains(got, "Posts about model trains.") {
		t.Errorf("a first name did not find the person: %q", got)
	}
}

func TestAnAliasFindsThePerson(t *testing.T) {
	doc := "## People\n\n### Steve Steveson\n<!-- aka: the Facebook guy -->\n- Posts about model trains.\n"

	got := recalled(doc, "[12:00] drew: do the Facebook guy's voice")

	if !strings.Contains(got, "Posts about model trains.") {
		t.Errorf("an alias did not find the person: %q", got)
	}
}

func TestAShortOrOrdinaryWordDoesNotDragEverybodyIn(t *testing.T) {
	// A person called "Will" or aliased "guy" would otherwise be In Play in
	// every conversation in the server.
	doc := "## People\n\n### Al\n- Fixes bikes.\n\n### Steve\n<!-- aka: the guy -->\n- Hates boats.\n"

	got := recalled(doc, "[12:00] drew: that guy will be along later, all of them")

	if strings.Contains(got, "Fixes bikes.") {
		t.Errorf("a two-letter name matched ordinary prose: %q", got)
	}
	if strings.Contains(got, "Hates boats.") {
		t.Errorf("an ordinary word matched as an alias: %q", got)
	}
}

func TestSomebodyWhoSpokeIsFoundByTheirSnowflakeAfterARename(t *testing.T) {
	// The Backlog shows their new name, the notes are headed with the old one.
	// The snowflake is the only handle that spans the two.
	doc := "## People\n\n### drew\n<!-- id: 424242 -->\n- Runs the server.\n"

	got := recall(parseMemory(doc), "[12:00] Drewseph: anyone about?",
		[]namedUser{{ID: "424242", Name: "Drewseph"}}, 8<<10)

	if !strings.Contains(got, "Runs the server.") {
		t.Errorf("a renamed namedUser was not recalled: %q", got)
	}
}

func TestTheBudgetKeepsWhoeverCameUpLast(t *testing.T) {
	doc := "## People\n\n### Steve\n- " + strings.Repeat("boats ", 30) + "\n\n" +
		"### Kate\n- " + strings.Repeat("cabin ", 30) + "\n"

	// Room for one of them, and Kate is the one being talked about now.
	got := recall(parseMemory(doc), "[12:00] drew: steve said\n[12:01] drew: what about kate", nil, 260)

	if !strings.Contains(got, "cabin") {
		t.Errorf("the most recently mentioned person was dropped: %q", got)
	}
	if strings.Contains(got, "boats") {
		t.Errorf("the budget was overrun: %q", got)
	}
}

func TestSomebodyCutForSpaceStillAppearsAsAName(t *testing.T) {
	doc := "## People\n\n### Steve\n- " + strings.Repeat("boats ", 30) + "\n\n" +
		"### Kate\n- " + strings.Repeat("cabin ", 30) + "\n"

	got := recall(parseMemory(doc), "[12:00] drew: steve said\n[12:01] drew: what about kate", nil, 260)

	if !strings.Contains(got, "Steve") {
		t.Errorf("a person cut for space vanished entirely: %q", got)
	}
}

func TestOnePersonTooLargeForTheBudgetKeepsTheirNewestNotes(t *testing.T) {
	// Nothing is forgotten on disk; this is only what fits in one prompt. Newest
	// first, because recent material is the more representative of how somebody
	// is right now.
	doc := "## People\n\n### Steve\n- the oldest thing ever recorded about him\n" +
		"- " + strings.Repeat("filler ", 40) + "\n- the newest thing recorded about him\n"

	got := recall(parseMemory(doc), "[12:00] drew: what about steve", nil, 200)

	if !strings.Contains(got, "the newest thing recorded about him") {
		t.Errorf("the newest note was truncated away: %q", got)
	}
	if strings.Contains(got, "the oldest thing ever recorded about him") {
		t.Errorf("the budget was overrun: %q", got)
	}
	if !strings.Contains(got, "not shown") {
		t.Errorf("truncation should be visible, or BeanBot denies notes he has: %q", got)
	}
}

func TestAnEmptyRosterAddsNothingToThePrompt(t *testing.T) {
	// An empty People heading invites the model to announce that it knows
	// nobody.
	got := recalled("## Traditions\n- Thursday is game night.\n", "[12:00] drew: hi")

	if strings.Contains(got, rosterHeading) {
		t.Errorf("an empty roster produced a heading: %q", got)
	}
	if strings.Contains(got, "Other people") {
		t.Errorf("an empty roster produced an index: %q", got)
	}
}

func TestBulletsOrphanedUnderPeopleAreAlwaysRecalled(t *testing.T) {
	// They belong to nobody, so nothing can decide they are In Play. There are
	// few of them and they are small.
	doc := "## People\n- Something written before people had headings.\n\n### Kate\n- Restores arcade cabinets.\n"

	got := recalled(doc, "[12:00] drew: hi")

	if !strings.Contains(got, "Something written before people had headings.") {
		t.Errorf("an orphaned bullet never reached the prompt: %q", got)
	}
}

func TestTheSnowflakeIsNotWorthPromptSpace(t *testing.T) {
	// It is how Go finds a Person across a rename; the model has no use for it
	// and cannot @-mention with it.
	doc := "## People\n\n### drew\n<!-- id: 424242; aka: Drewseph -->\n- Runs the server.\n"

	got := recalled(doc, "[12:00] someone: is drew about?")

	if strings.Contains(got, "424242") {
		t.Errorf("the snowflake was spent on prompt space: %q", got)
	}
	if !strings.Contains(got, "Drewseph") {
		t.Errorf("the aliases should be shown, they are what he is called: %q", got)
	}
}

func TestSomebodyWhoseNameIsAnOrdinaryWordIsStillFound(t *testing.T) {
	// The ordinary-word filter exists to stop "the Facebook guy" turning on
	// "guy". Applied to somebody actually called Guy it would make them
	// permanently unfindable, which is a worse failure than a noisy match.
	doc := "## People\n\n### Guy\n- Fixes bikes.\n"

	got := recalled(doc, "[12:00] drew: has Guy looked at it yet")

	if !strings.Contains(got, "Fixes bikes.") {
		t.Errorf("a person named Guy could not be found by name: %q", got)
	}
}
