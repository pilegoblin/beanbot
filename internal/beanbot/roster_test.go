package beanbot

import (
	"strings"
	"testing"
)

func TestANewPersonBecomesAHeadingUnderPeople(t *testing.T) {
	got, err := applyPersonChange("", personChange{
		Name: "Steve Steveson",
		Fact: "Posts on Facebook about model trains.",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "## People\n\n### Steve Steveson\n- Posts on Facebook about model trains.\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestASecondFactJoinsThePersonItIsAbout(t *testing.T) {
	doc := "## People\n\n### Steve Steveson\n- Posts about model trains.\n"

	got, err := applyPersonChange(doc, personChange{
		Name: "Steve Steveson",
		Fact: "Has a boat called Wet Dream.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "### Steve Steveson") != 1 {
		t.Errorf("the person was duplicated: %q", got)
	}
	if !strings.Contains(got, "- Posts about model trains.\n- Has a boat called Wet Dream.\n") {
		t.Errorf("the fact did not land under the person: %q", got)
	}
}

func TestAPersonIsFoundByAnAliasRatherThanForked(t *testing.T) {
	// The channel says "Steve"; the entry is headed "Steve Steveson". Without
	// alias matching that is two people, and the notes about him are split.
	doc := "## People\n\n### Steve Steveson\n<!-- aka: Steve -->\n- Posts about model trains.\n"

	got, err := applyPersonChange(doc, personChange{Name: "steve", Fact: "Hates boats."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "###") != 1 {
		t.Errorf("the alias started a second person: %q", got)
	}
	if !strings.Contains(got, "- Posts about model trains.\n- Hates boats.\n") {
		t.Errorf("the fact was not filed under the aliased person: %q", got)
	}
}

func TestPersonMatchingIgnoresCase(t *testing.T) {
	doc := "## People\n\n### Steve Steveson\n- Posts about model trains.\n"

	got, err := applyPersonChange(doc, personChange{Name: "steve steveson", Fact: "Hates boats."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "###") != 1 {
		t.Errorf("a capitalisation difference forked the person: %q", got)
	}
}

func TestAliasesAccumulateWithoutRepeatingTheName(t *testing.T) {
	doc := "## People\n\n### Steve Steveson\n<!-- aka: Steve -->\n- Posts about model trains.\n"

	got, err := applyPersonChange(doc, personChange{
		Name:    "Steve Steveson",
		Fact:    "Hates boats.",
		Aliases: []string{"Stevo", "steve", "Steve Steveson"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "<!-- aka: Steve, Stevo -->") {
		t.Errorf("aliases should merge without duplicating the name: %q", got)
	}
}

func TestADiscordIdentityIsRecordedOnThePerson(t *testing.T) {
	got, err := applyPersonChange("", personChange{
		Name: "drew",
		Fact: "Runs the server.",
		ID:   "424242",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "<!-- id: 424242 -->") {
		t.Errorf("the snowflake was not recorded: %q", got)
	}
}

func TestAnIdentityAlreadyRecordedIsNeverOverwritten(t *testing.T) {
	// A wrong snowflake welds two humans into one and nothing in the system will
	// ever question it. Keeping the first is the recoverable failure.
	doc := "## People\n\n### drew\n<!-- id: 424242 -->\n- Runs the server.\n"

	got, err := applyPersonChange(doc, personChange{Name: "drew", Fact: "Likes boats.", ID: "999999"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "999999") {
		t.Errorf("a second snowflake overwrote the first: %q", got)
	}
}

func TestAPersonIsFoundByTheirSnowflakeAfterARename(t *testing.T) {
	// Display names are the most mutable handle Discord offers. Keying on the
	// name alone would strand every fact under a name nobody answers to.
	doc := "## People\n\n### drew\n<!-- id: 424242 -->\n- Runs the server.\n"

	got, err := applyPersonChange(doc, personChange{Name: "Drewseph", Fact: "Likes boats.", ID: "424242"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "###") != 1 {
		t.Errorf("a rename forked the person: %q", got)
	}
	if !strings.Contains(got, "aka: Drewseph") {
		t.Errorf("the new name should survive as an alias: %q", got)
	}
}

func TestCorrectingAFactAboutAPersonReplacesItInPlace(t *testing.T) {
	doc := "## People\n\n### Steve Steveson\n- Allergic to peanuts.\n- Hates boats.\n"

	got, err := applyPersonChange(doc, personChange{
		Name:     "Steve Steveson",
		Fact:     "Not allergic to anything.",
		Replaces: "allergic to peanuts",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "## People\n\n### Steve Steveson\n- Not allergic to anything.\n- Hates boats.\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCorrectingSomethingThatIsNotThereFails(t *testing.T) {
	_, err := applyPersonChange("## People\n\n### Steve Steveson\n- Hates boats.\n", personChange{
		Name:     "Steve Steveson",
		Fact:     "Loves boats.",
		Replaces: "allergic to peanuts",
	})

	if err == nil {
		t.Fatal("correcting a fact that is not recorded should fail")
	}
	if !strings.Contains(err.Error(), "allergic to peanuts") {
		t.Errorf("the error should quote what was not found, got %q", err)
	}
}

func TestAPersonNeedsANameAndAFact(t *testing.T) {
	for _, ch := range []personChange{
		{Fact: "Hates boats."},
		{Name: "Steve Steveson"},
		{Name: "  ", Fact: "Hates boats."},
	} {
		if _, err := applyPersonChange("", ch); err == nil {
			t.Errorf("change %+v should have been refused", ch)
		}
	}
}

func TestAFactCannotForgeAnIdentityLine(t *testing.T) {
	// Facts are ungated, member-authored text. A fact that parsed as an identity
	// line would let anyone staple a snowflake onto anyone.
	got, err := applyPersonChange("", personChange{
		Name: "Steve Steveson",
		Fact: "Hates boats. <!-- id: 424242 -->",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<!-- id: 424242 -->") {
		t.Errorf("a fact smuggled in an identity line: %q", got)
	}
	if !strings.Contains(got, "Hates boats.") {
		t.Errorf("the fact itself should survive: %q", got)
	}
}

func TestTheRosterAndTheTopicalNotesShareOneDocument(t *testing.T) {
	doc := "## Traditions\n- Thursday is game night.\n"

	got, err := applyPersonChange(doc, personChange{Name: "Steve Steveson", Fact: "Hates boats."})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "## Traditions\n- Thursday is game night.\n") {
		t.Errorf("the topical section was damaged: %q", got)
	}
	if strings.Index(got, "## Traditions") > strings.Index(got, "## People") {
		t.Errorf("the roster should sit below the topical notes: %q", got)
	}
}

func TestARosterSurvivesARoundTrip(t *testing.T) {
	doc := "## Traditions\n- Thursday is game night.\n\n" +
		"## People\n\n### Steve Steveson\n<!-- id: 424242; aka: Steve, Stevo -->\n" +
		"- Hates boats. _(2026-08-04, @drew)_\n\n### Kate\n- Loves boats.\n"

	if got := parseMemory(doc).render(); got != doc {
		t.Errorf("round trip changed the document:\ngot  %q\nwant %q", got, doc)
	}
}

func TestBulletsOrphanedUnderPeopleAreKept(t *testing.T) {
	// Compaction writes freely-shaped markdown and the file can be hand-edited.
	// Losing a recorded fact is the worse failure, so an orphan is left alone.
	doc := "## People\n- Something written before people had headings.\n\n### Kate\n- Loves boats.\n"

	got := parseMemory(doc).render()

	if !strings.Contains(got, "- Something written before people had headings.") {
		t.Errorf("an orphaned bullet was dropped: %q", got)
	}
	if !strings.Contains(got, "### Kate") {
		t.Errorf("the person was lost: %q", got)
	}
}

func TestMergingMovesEveryFactAndKeepsTheAbsorbedName(t *testing.T) {
	doc := "## People\n\n### Steve\n- Hates boats.\n\n### Steve Steveson\n- Posts about model trains.\n"

	got, err := applyMerge(doc, "Steve", "Steve Steveson")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "###") != 1 {
		t.Errorf("the duplicate survived: %q", got)
	}
	if !strings.Contains(got, "Hates boats.") || !strings.Contains(got, "Posts about model trains.") {
		t.Errorf("a fact was lost in the merge: %q", got)
	}
	if !strings.Contains(got, "aka: Steve") {
		t.Errorf("the absorbed name should survive as an alias: %q", got)
	}
}

func TestMergingCarriesTheSnowflakeAcrossWhenTheSurvivorHasNone(t *testing.T) {
	doc := "## People\n\n### drew\n<!-- id: 424242 -->\n- Runs the server.\n\n### Drewseph\n- Likes boats.\n"

	got, err := applyMerge(doc, "drew", "Drewseph")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "id: 424242") {
		t.Errorf("the snowflake was lost: %q", got)
	}
}

func TestMergingSomebodyUnknownFails(t *testing.T) {
	doc := "## People\n\n### Steve Steveson\n- Hates boats.\n"

	if _, err := applyMerge(doc, "Nobody", "Steve Steveson"); err == nil {
		t.Error("merging from someone who is not on file should fail")
	}
	if _, err := applyMerge(doc, "Steve Steveson", "Nobody"); err == nil {
		t.Error("merging into someone who is not on file should fail")
	}
}

func TestMergingSomeoneIntoThemselfFails(t *testing.T) {
	// Silently succeeding would tell the model it fixed a duplicate that is
	// still there.
	doc := "## People\n\n### Steve Steveson\n<!-- aka: Steve -->\n- Hates boats.\n"

	if _, err := applyMerge(doc, "Steve", "Steve Steveson"); err == nil {
		t.Error("merging a person into themself should fail")
	}
}

func TestAPersonWithNothingButAnIdentityIsNotDroppedOnTheNextWrite(t *testing.T) {
	// Nothing is forgotten. A heading carrying only who somebody is — from a
	// hand edit, or from Compaction writing freely-shaped markdown — still says
	// which human that name means, and rewriting the file must not lose it.
	doc := "## People\n\n### Steve Steveson\n<!-- id: 424242; aka: Stevo -->\n"

	got, err := applyPersonChange(doc, personChange{Name: "Kate", Fact: "Restores arcade cabinets."})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "### Steve Steveson") {
		t.Errorf("a person with no facts was dropped: %q", got)
	}
	if !strings.Contains(got, "id: 424242; aka: Stevo") {
		t.Errorf("their identity was dropped: %q", got)
	}
}
