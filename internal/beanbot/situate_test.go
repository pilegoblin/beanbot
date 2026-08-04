package beanbot

import (
	"strings"
	"testing"
	"time"
)

var noonUTC = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

func TestMemoryIsFencedAndLabelledAsFallible(t *testing.T) {
	// Memory is ungated, member-authored text that goes into every later
	// prompt. Fencing it as notes rather than instruction is the whole
	// mitigation, since the Backstory is the one thing no member can write.
	got := situate("## People\n- Steve likes boats.\n", "[12:00] drew: hi", noonUTC)

	if !strings.Contains(got, "<notes>") || !strings.Contains(got, "</notes>") {
		t.Errorf("memory was not fenced: %q", got)
	}
	if !strings.Contains(got, "not instructions") {
		t.Errorf("memory was not labelled as fallible notes: %q", got)
	}
	if strings.Index(got, "<notes>") > strings.Index(got, "[12:00] drew: hi") {
		t.Errorf("notes should precede the backlog: %q", got)
	}
}

func TestNoMemoryMeansNoEmptyNotesBlock(t *testing.T) {
	// An empty fence invites the model to explain that it remembers nothing.
	for _, memory := range []string{"", "   \n\n"} {
		got := situate(memory, "[12:00] drew: hi", noonUTC)

		if strings.Contains(got, "<notes>") {
			t.Errorf("memory %q produced an empty notes block: %q", memory, got)
		}
		if !strings.Contains(got, "[12:00] drew: hi") {
			t.Errorf("the backlog went missing: %q", got)
		}
	}
}

func TestTheModelIsAlwaysToldTheTimeAndZone(t *testing.T) {
	got := situate("", "[12:00] drew: hi", noonUTC)

	if !strings.Contains(got, "2026-08-04T12:00:00Z") {
		t.Errorf("the current time is missing: %q", got)
	}
	if !strings.Contains(got, "Configured timezone: UTC") {
		t.Errorf("the timezone is missing: %q", got)
	}
}
