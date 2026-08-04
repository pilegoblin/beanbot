package beanbot

import (
	"strings"
	"testing"
)

func TestRecordingIntoAnEmptyMemoryCreatesTheSection(t *testing.T) {
	got, err := applyChange("", change{Section: "Traditions", Claim: "Thursday is game night."})
	if err != nil {
		t.Fatal(err)
	}

	want := "## Traditions\n- Thursday is game night.\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestASecondEntryJoinsTheSectionItBelongsTo(t *testing.T) {
	doc := "## Traditions\n- Thursday is game night.\n"

	got, err := applyChange(doc, change{Section: "Traditions", Claim: "Sunday is a roast."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "## Traditions") != 1 {
		t.Errorf("the section was duplicated: %q", got)
	}
	if !strings.Contains(got, "- Thursday is game night.\n- Sunday is a roast.\n") {
		t.Errorf("the entry did not land under the heading: %q", got)
	}
}

func TestSectionMatchingIgnoresCase(t *testing.T) {
	// The model retypes the heading from what it was shown and will not always
	// match the capitalisation. Two "Traditions" sections is a split memory.
	doc := "## Traditions\n- Thursday is game night.\n"

	got, err := applyChange(doc, change{Section: "traditions", Claim: "Sunday is a roast."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(strings.ToLower(got), "## traditions") != 1 {
		t.Errorf("case difference split the section: %q", got)
	}
}

func TestAnUnknownSectionIsAppendedAfterTheExistingOnes(t *testing.T) {
	doc := "## Traditions\n- Thursday is game night.\n"

	got, err := applyChange(doc, change{Section: "Running jokes", Claim: "Nobody explains the sandwich."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Index(got, "## Traditions") > strings.Index(got, "## Running jokes") {
		t.Errorf("the new section jumped ahead of the existing one: %q", got)
	}
	if !strings.Contains(got, "## Running jokes\n- Nobody explains the sandwich.\n") {
		t.Errorf("the new section is malformed: %q", got)
	}
}

func TestSupersedingAnEntryReplacesItInPlace(t *testing.T) {
	doc := "## Traditions\n- Game night is on Thursdays.\n- Sunday is a roast.\n"

	got, err := applyChange(doc, change{
		Section:  "Traditions",
		Claim:    "Game night moved to Fridays.",
		Replaces: "Game night is on Thursdays",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "on Thursdays") {
		t.Errorf("the superseded entry survived: %q", got)
	}
	// Position is preserved so an ordinary correction does not reshuffle the
	// document and hand compaction gratuitous churn.
	want := "## Traditions\n- Game night moved to Fridays.\n- Sunday is a roast.\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSupersedingMatchesOnASubstringOfTheEntry(t *testing.T) {
	// Entries carry a trailing attribution the model is unlikely to reproduce
	// exactly, so an exact-match requirement would make correction unusable.
	doc := "## Traditions\n- Game night is on Thursdays. _(2026-03-02, @steve)_\n"

	got, err := applyChange(doc, change{
		Section:  "Traditions",
		Claim:    "Game night moved to Fridays. _(2026-08-04, @kate)_",
		Replaces: "game night is   ON thursdays",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "Thursdays") {
		t.Errorf("the superseded entry survived: %q", got)
	}
}

func TestSupersedingSomethingThatIsNotThereFails(t *testing.T) {
	// Reported back to the model, which can then retry as a plain addition
	// rather than silently recording a correction to nothing.
	_, err := applyChange("## Traditions\n- Sunday is a roast.\n", change{
		Section:  "Traditions",
		Claim:    "Game night moved to Fridays.",
		Replaces: "Game night is on Thursdays",
	})

	if err == nil {
		t.Fatal("replacing a nonexistent entry should fail")
	}
	if !strings.Contains(err.Error(), "Game night is on Thursdays") {
		t.Errorf("the error should quote what was not found, got %q", err)
	}
}

func TestSupersedingAnEntryFiledElsewhereMovesIt(t *testing.T) {
	// Re-filing is how a miscategorised memory gets fixed; leaving the original
	// behind would duplicate the fact into two sections.
	doc := "## Running jokes\n- Thursday is game night.\n\n## Traditions\n- Nobody explains the sandwich.\n"

	got, err := applyChange(doc, change{
		Section:  "Traditions",
		Claim:    "Thursday is game night.",
		Replaces: "Thursday is game night",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "Thursday is game night") != 1 {
		t.Errorf("the moved entry was duplicated: %q", got)
	}
	if strings.Contains(got, "## Running jokes") {
		t.Errorf("the emptied section should have been dropped: %q", got)
	}
}

func TestAnEntryWithNoSectionIsRefused(t *testing.T) {
	if _, err := applyChange("", change{Claim: "Thursday is game night."}); err == nil {
		t.Error("a change with no section should fail")
	}
}

func TestATopicalNoteMayNotBeFiledUnderPeople(t *testing.T) {
	// Otherwise what BeanBot knows about somebody splits between a heading with
	// their name on it and a loose bullet nothing can find. The refusal is
	// reported back so the model retries through remember_person.
	_, err := applyChange("", change{Section: "people", Claim: "Steve hates boats."})

	if err == nil {
		t.Fatal("the People section is reserved for the Roster")
	}
	if !strings.Contains(err.Error(), "remember_person") {
		t.Errorf("the refusal should name the tool to use instead, got %q", err)
	}
}

func TestAnEmptySectionIsRefused(t *testing.T) {
	if _, err := applyChange("", change{Section: "Traditions", Claim: "   "}); err == nil {
		t.Error("a change with no entry should fail")
	}
}

func TestAWellFormedDocumentSurvivesARoundTrip(t *testing.T) {
	doc := "## Traditions\n- Thursday is game night.\n\n## Running jokes\n- Nobody explains the sandwich.\n"

	if got := parseMemory(doc).render(); got != doc {
		t.Errorf("round trip changed the document:\ngot  %q\nwant %q", got, doc)
	}
}

func TestProseWrittenByCompactionIsNotDiscarded(t *testing.T) {
	// Compaction is a language model writing this file, so it will not always
	// produce clean bullets. Losing a recorded fact is worse than keeping an
	// oddly shaped one.
	doc := "## Traditions\n- Thursday is game night.\n  It has been since 2019.\n"

	got := parseMemory(doc).render()

	if !strings.Contains(got, "It has been since 2019.") {
		t.Errorf("a continuation line was dropped: %q", got)
	}
}

func TestTextAboveTheFirstHeadingIsKept(t *testing.T) {
	doc := "Notes about this server.\n\n## Traditions\n- Thursday is game night.\n"

	got := parseMemory(doc).render()

	if !strings.HasPrefix(got, "Notes about this server.") {
		t.Errorf("the preamble was lost or moved: %q", got)
	}
	if !strings.Contains(got, "## Traditions") {
		t.Errorf("the section was lost: %q", got)
	}
}
