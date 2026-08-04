package beanbot

import (
	"context"
	"strings"
	"testing"
)

func personArgs(name, fact string) map[string]any {
	return map[string]any{"name": name, "fact": fact}
}

func TestANoteAboutSomebodyIsFiledUnderTheirName(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	if _, err := (rememberPerson{memory: m}).Execute(context.Background(),
		memoryExecution(personArgs("Steve Steveson", "Posts on Facebook about model trains."))); err != nil {
		t.Fatal(err)
	}

	got := m.Load("424242")
	if !strings.Contains(got, "### Steve Steveson") {
		t.Errorf("the person got no entry of their own: %q", got)
	}
	if !strings.Contains(got, "Posts on Facebook about model trains.") {
		t.Errorf("the fact was not recorded: %q", got)
	}
	if !strings.Contains(got, "_(2026-08-04, @drew)_") {
		t.Errorf("the attribution is missing: %q", got)
	}
}

func TestSomebodyOnlyTalkedAboutNeedsNoDiscordAccount(t *testing.T) {
	// The whole point: Steve Steveson is not in this server and never will be.
	m := openTestMemory(t, 8<<10, nil)

	if _, err := (rememberPerson{memory: m}).Execute(context.Background(),
		memoryExecution(personArgs("Steve Steveson", "Posts about model trains."))); err != nil {
		t.Fatal(err)
	}

	if got := m.Load("424242"); strings.Contains(got, "id:") {
		t.Errorf("a snowflake was invented for somebody who has none: %q", got)
	}
}

func TestWritingAboutYourselfRecordsYourSnowflake(t *testing.T) {
	// Discord stated this one: the Trigger came from them. No inference, so no
	// way to staple the wrong human's identity onto the entry.
	m := openTestMemory(t, 8<<10, nil)

	inv := memoryExecution(personArgs("drew", "Runs the server."))
	inv.UserID = "777"

	if _, err := (rememberPerson{memory: m}).Execute(context.Background(), inv); err != nil {
		t.Fatal(err)
	}

	if got := m.Load("424242"); !strings.Contains(got, "id: 777") {
		t.Errorf("the caller's own snowflake was not recorded: %q", got)
	}
}

func TestWritingAboutSomebodyMentionedRecordsTheirSnowflake(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	inv := memoryExecution(personArgs("kate", "Restores arcade cabinets."))
	inv.Mentions = []namedUser{{ID: "888", Name: "kate"}}

	if _, err := (rememberPerson{memory: m}).Execute(context.Background(), inv); err != nil {
		t.Fatal(err)
	}

	if got := m.Load("424242"); !strings.Contains(got, "id: 888") {
		t.Errorf("a resolved mention's snowflake was not recorded: %q", got)
	}
}

func TestSomebodyNeitherMentionedNorWritingGetsNoSnowflake(t *testing.T) {
	// A wrong snowflake welds two humans together and nothing downstream will
	// ever question it. A Steve in the channel is not the Steve on Facebook.
	m := openTestMemory(t, 8<<10, nil)

	inv := memoryExecution(personArgs("Steve", "Posts about model trains."))
	inv.Mentions = []namedUser{{ID: "888", Name: "kate"}}

	if _, err := (rememberPerson{memory: m}).Execute(context.Background(), inv); err != nil {
		t.Fatal(err)
	}

	if got := m.Load("424242"); strings.Contains(got, "id:") {
		t.Errorf("a snowflake was guessed at: %q", got)
	}
}

func TestAliasesArriveFromTheModelAsAList(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	args := personArgs("Steve Steveson", "Posts about model trains.")
	args["aliases"] = []any{"Stevo", "the Facebook guy"}

	if _, err := (rememberPerson{memory: m}).Execute(context.Background(), memoryExecution(args)); err != nil {
		t.Fatal(err)
	}

	if got := m.Load("424242"); !strings.Contains(got, "aka: Stevo, the Facebook guy") {
		t.Errorf("the aliases were not recorded: %q", got)
	}
}

func TestANoteAboutSomebodyOutsideAServerIsRefused(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	inv := memoryExecution(personArgs("Steve Steveson", "Posts about model trains."))
	inv.GuildID = ""

	if _, err := (rememberPerson{memory: m}).Execute(context.Background(), inv); err == nil {
		t.Error("a Trigger with no guild has no Roster to write to")
	}
}

func TestANoteAboutSomebodyNeedsANameAndAFact(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)
	r := rememberPerson{memory: m}

	for _, args := range []map[string]any{
		{"fact": "Posts about model trains."},
		{"name": "Steve Steveson"},
		{"name": "Steve Steveson", "fact": ""},
	} {
		if _, err := r.Execute(context.Background(), memoryExecution(args)); err == nil {
			t.Errorf("args %v should have been refused", args)
		}
	}
}

func TestAnyoneMayTeachBeanbotAboutSomebody(t *testing.T) {
	// Same reasoning as remember: a Roster only moderators may write to is a
	// mod-maintained wiki, not BeanBot learning who the server talks about.
	if got := (rememberPerson{}).RequiredPermission(); got != 0 {
		t.Errorf("remember_person should be ungated, requires %#x", got)
	}
	if (rememberPerson{}).Mutating() {
		t.Error("a note about somebody changes nothing in the guild")
	}
}

func TestMergingFoldsADuplicateIntoTheRealPerson(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)
	r := rememberPerson{memory: m}

	if _, err := r.Execute(context.Background(),
		memoryExecution(personArgs("Steve", "Hates boats."))); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Execute(context.Background(),
		memoryExecution(personArgs("Steve Steveson", "Posts about model trains."))); err != nil {
		t.Fatal(err)
	}

	if _, err := (mergePeople{memory: m}).Execute(context.Background(),
		memoryExecution(map[string]any{"from": "Steve", "into": "Steve Steveson"})); err != nil {
		t.Fatal(err)
	}

	got := m.Load("424242")
	if strings.Count(got, "###") != 1 {
		t.Errorf("the duplicate survived the merge: %q", got)
	}
	if !strings.Contains(got, "Hates boats.") || !strings.Contains(got, "Posts about model trains.") {
		t.Errorf("a fact was lost in the merge: %q", got)
	}
}

func TestMergingIsUngatedBecauseItDestroysNothing(t *testing.T) {
	// Every fact moves across and the absorbed name survives as an alias, so the
	// worst a merge can do is weld two people together — visible, and repairable.
	if got := (mergePeople{}).RequiredPermission(); got != 0 {
		t.Errorf("merge_people should be ungated, requires %#x", got)
	}
	if (mergePeople{}).Mutating() {
		t.Error("merging notes changes nothing in the guild")
	}
}

func TestMergingSomebodyBeanbotHasNoNotesAboutIsReported(t *testing.T) {
	// Reported back to the model rather than silently succeeding, so it does not
	// announce that it tidied up a duplicate that is still there.
	m := openTestMemory(t, 8<<10, nil)

	if _, err := (mergePeople{memory: m}).Execute(context.Background(),
		memoryExecution(map[string]any{"from": "Nobody", "into": "Steve Steveson"})); err == nil {
		t.Error("merging someone who is not on file should fail")
	}
}
