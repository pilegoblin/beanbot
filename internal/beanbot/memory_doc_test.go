package beanbot

import (
	"strings"
	"testing"
)

func TestRecordingIntoAnEmptyMemoryCreatesTheSection(t *testing.T) {
	got, err := applyChange("", change{Section: "People", Entry: "Steve likes boats."})
	if err != nil {
		t.Fatal(err)
	}

	want := "## People\n- Steve likes boats.\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestASecondEntryJoinsTheSectionItBelongsTo(t *testing.T) {
	doc := "## People\n- Steve likes boats.\n"

	got, err := applyChange(doc, change{Section: "People", Entry: "Kate hates boats."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "## People") != 1 {
		t.Errorf("the section was duplicated: %q", got)
	}
	if !strings.Contains(got, "- Steve likes boats.\n- Kate hates boats.\n") {
		t.Errorf("the entry did not land under the heading: %q", got)
	}
}

func TestSectionMatchingIgnoresCase(t *testing.T) {
	// The model retypes the heading from what it was shown and will not always
	// match the capitalisation. Two "People" sections is a split memory.
	doc := "## People\n- Steve likes boats.\n"

	got, err := applyChange(doc, change{Section: "people", Entry: "Kate hates boats."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(strings.ToLower(got), "## people") != 1 {
		t.Errorf("case difference split the section: %q", got)
	}
}

func TestAnUnknownSectionIsAppendedAfterTheExistingOnes(t *testing.T) {
	doc := "## People\n- Steve likes boats.\n"

	got, err := applyChange(doc, change{Section: "Traditions", Entry: "Thursday is game night."})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Index(got, "## People") > strings.Index(got, "## Traditions") {
		t.Errorf("the new section jumped ahead of the existing one: %q", got)
	}
	if !strings.Contains(got, "## Traditions\n- Thursday is game night.\n") {
		t.Errorf("the new section is malformed: %q", got)
	}
}

func TestSupersedingAnEntryReplacesItInPlace(t *testing.T) {
	doc := "## People\n- Steve is allergic to peanuts.\n- Kate hates boats.\n"

	got, err := applyChange(doc, change{
		Section:  "People",
		Entry:    "Steve is not allergic to anything.",
		Replaces: "Steve is allergic to peanuts",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "allergic to peanuts") {
		t.Errorf("the superseded entry survived: %q", got)
	}
	// Position is preserved so an ordinary correction does not reshuffle the
	// document and hand compaction gratuitous churn.
	want := "## People\n- Steve is not allergic to anything.\n- Kate hates boats.\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSupersedingMatchesOnASubstringOfTheEntry(t *testing.T) {
	// Entries carry a trailing attribution the model is unlikely to reproduce
	// exactly, so an exact-match requirement would make correction unusable.
	doc := "## People\n- Steve is allergic to peanuts. _(2026-03-02, @steve)_\n"

	got, err := applyChange(doc, change{
		Section:  "People",
		Entry:    "Steve is fine with peanuts. _(2026-08-04, @kate)_",
		Replaces: "steve is   ALLERGIC to peanuts",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "allergic") {
		t.Errorf("the superseded entry survived: %q", got)
	}
}

func TestSupersedingSomethingThatIsNotThereFails(t *testing.T) {
	// Reported back to the model, which can then retry as a plain addition
	// rather than silently recording a correction to nothing.
	_, err := applyChange("## People\n- Kate hates boats.\n", change{
		Section:  "People",
		Entry:    "Steve is fine with peanuts.",
		Replaces: "Steve is allergic to peanuts",
	})

	if err == nil {
		t.Fatal("replacing a nonexistent entry should fail")
	}
	if !strings.Contains(err.Error(), "Steve is allergic to peanuts") {
		t.Errorf("the error should quote what was not found, got %q", err)
	}
}

func TestSupersedingAnEntryFiledElsewhereMovesIt(t *testing.T) {
	// Re-filing is how a miscategorised memory gets fixed; leaving the original
	// behind would duplicate the fact into two sections.
	doc := "## People\n- Thursday is game night.\n\n## Traditions\n- Nobody explains the sandwich.\n"

	got, err := applyChange(doc, change{
		Section:  "Traditions",
		Entry:    "Thursday is game night.",
		Replaces: "Thursday is game night",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "Thursday is game night") != 1 {
		t.Errorf("the moved entry was duplicated: %q", got)
	}
	if strings.Contains(got, "## People") {
		t.Errorf("the emptied section should have been dropped: %q", got)
	}
}

func TestAnEntryWithNoSectionIsRefused(t *testing.T) {
	if _, err := applyChange("", change{Entry: "Steve likes boats."}); err == nil {
		t.Error("a change with no section should fail")
	}
}

func TestAnEmptySectionIsRefused(t *testing.T) {
	if _, err := applyChange("", change{Section: "People", Entry: "   "}); err == nil {
		t.Error("a change with no entry should fail")
	}
}

func TestAWellFormedDocumentSurvivesARoundTrip(t *testing.T) {
	doc := "## People\n- Steve likes boats.\n\n## Traditions\n- Thursday is game night.\n"

	if got := parseMemory(doc).render(); got != doc {
		t.Errorf("round trip changed the document:\ngot  %q\nwant %q", got, doc)
	}
}

func TestProseWrittenByCompactionIsNotDiscarded(t *testing.T) {
	// Compaction is a language model writing this file, so it will not always
	// produce clean bullets. Losing a recorded fact is worse than keeping an
	// oddly shaped one.
	doc := "## People\n- Steve likes boats.\n  He owns three of them.\n"

	got := parseMemory(doc).render()

	if !strings.Contains(got, "He owns three of them.") {
		t.Errorf("a continuation line was dropped: %q", got)
	}
}

func TestTextAboveTheFirstHeadingIsKept(t *testing.T) {
	doc := "Notes about this server.\n\n## People\n- Steve likes boats.\n"

	got := parseMemory(doc).render()

	if !strings.HasPrefix(got, "Notes about this server.") {
		t.Errorf("the preamble was lost or moved: %q", got)
	}
	if !strings.Contains(got, "## People") {
		t.Errorf("the section was lost: %q", got)
	}
}
