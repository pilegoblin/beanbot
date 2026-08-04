package beanbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pilegoblin/beanbot/internal/gemini"
)

// memoryExecution is execution() with a guild id that is a real snowflake,
// since the id becomes a filename.
func memoryExecution(args map[string]any) Execution {
	inv := execution(&fakeGuild{}, args)
	inv.GuildID = "424242"
	inv.Author = "drew"
	inv.Now = time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)
	return inv
}

func rememberArgs(section, entry string) map[string]any {
	return map[string]any{"section": section, "entry": entry}
}

func TestARememberedThingIsWrittenDownWithWhoAndWhen(t *testing.T) {
	// Writes are ungated, so attribution is what lets someone reading the file
	// later tell a shared fact from one member's mischief.
	m := openTestMemory(t, 8<<10, nil)

	if _, err := (remember{memory: m}).Execute(context.Background(),
		memoryExecution(rememberArgs("People", "Steve likes boats."))); err != nil {
		t.Fatal(err)
	}

	got := m.Load("424242")
	if !strings.Contains(got, "## People") {
		t.Errorf("no section heading: %q", got)
	}
	if !strings.Contains(got, "Steve likes boats.") {
		t.Errorf("the fact was not recorded: %q", got)
	}
	if !strings.Contains(got, "_(2026-08-04, @drew)_") {
		t.Errorf("the attribution is missing: %q", got)
	}
}

func TestRememberingCorrectsInsteadOfContradicting(t *testing.T) {
	// There is no forget Capability, so superseding is the only way a wrong
	// note gets fixed before Compaction eventually runs.
	m := openTestMemory(t, 8<<10, nil)
	r := remember{memory: m}

	if _, err := r.Execute(context.Background(),
		memoryExecution(rememberArgs("People", "Steve is allergic to peanuts."))); err != nil {
		t.Fatal(err)
	}

	args := rememberArgs("People", "Steve is not allergic to anything.")
	args["replaces"] = "Steve is allergic to peanuts"
	if _, err := r.Execute(context.Background(), memoryExecution(args)); err != nil {
		t.Fatal(err)
	}

	got := m.Load("424242")
	if strings.Contains(got, "allergic to peanuts") {
		t.Errorf("both claims are now on file: %q", got)
	}
	if !strings.Contains(got, "not allergic to anything") {
		t.Errorf("the correction was not recorded: %q", got)
	}
}

func TestRememberingOutsideAServerIsRefused(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	inv := memoryExecution(rememberArgs("People", "Steve likes boats."))
	inv.GuildID = ""

	if _, err := (remember{memory: m}).Execute(context.Background(), inv); err == nil {
		t.Error("a Trigger with no guild has no Memory to write to")
	}
}

func TestRememberingNothingIsRefused(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)
	r := remember{memory: m}

	for _, args := range []map[string]any{
		{"entry": "Steve likes boats."},
		{"section": "People"},
		{"section": "People", "entry": ""},
	} {
		if _, err := r.Execute(context.Background(), memoryExecution(args)); err == nil {
			t.Errorf("args %v should have been refused", args)
		}
	}
}

func TestAnyoneMayTeachBeanbotSomething(t *testing.T) {
	// Gated on a permission, Memory would be a mod-maintained wiki rather than
	// BeanBot learning from the people in the server.
	if got := (remember{}).RequiredPermission(); got != 0 {
		t.Errorf("remember should be ungated, requires %#x", got)
	}
}

func TestRememberingComposesWithChangingTheGuild(t *testing.T) {
	// "make the event and remember we do this weekly" is one sentence. Sharing
	// the single Guild-mutation budget would half-refuse it.
	m := openTestMemory(t, 8<<10, nil)
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{
		callFor("mutate"),
		{FunctionCalls: []gemini.FunctionCall{{
			Name: "remember",
			Args: rememberArgs("Traditions", "Thursday is game night."),
		}}},
	}}
	a := newAgent([]Capability{
		countingCap{name: "mutate", mutating: true, runs: &runs},
		remember{memory: m},
	})

	if _, _, err := a.run(context.Background(), model, "backlog", nil, memoryExecution(nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if runs != 1 {
		t.Errorf("the guild-mutating capability ran %d times, want 1", runs)
	}
	if got := m.Load("424242"); !strings.Contains(got, "Thursday is game night.") {
		t.Errorf("the memory write was refused alongside the mutation: %q", got)
	}
}
