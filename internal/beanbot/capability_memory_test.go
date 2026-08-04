package beanbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pilegoblin/beanbot/internal/gemini"
)

// testSources are the numbered Backlog lines a test Execution may cite: an older
// line from somebody other than the caller, the caller's own, and BeanBot's.
var testSources = []source{
	{speaker: "kate", at: at("2026-08-01 14:02")},
	{speaker: "drew", at: at("14:00")},
	{speaker: "BeanBot", at: at("14:01"), mine: true},
}

// memoryExecution is execution() with a guild id that is a real snowflake,
// since the id becomes a filename.
func memoryExecution(args map[string]any) Execution {
	inv := execution(&fakeGuild{}, args)
	inv.GuildID = "424242"
	inv.Author = "drew"
	inv.Now = time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC)
	inv.Sources = testSources
	return inv
}

// cite adds the Source every note-taking Capability requires. Line 2 is the
// caller's own, which is the case that used to be the only one Go could express.
func cite(args map[string]any, line int, speaker string) map[string]any {
	args["said_in"] = line
	args["said_by"] = speaker
	return args
}

func rememberArgs(section, claim string) map[string]any {
	return cite(map[string]any{"section": section, "claim": claim}, 2, "drew")
}

func TestARememberedThingIsWrittenDownWithWhoAndWhen(t *testing.T) {
	// Writes are ungated, so Attribution is what lets someone reading the file
	// later tell what the server agreed on from what one member asserted once.
	m := openTestMemory(t, 8<<10, nil)

	if _, err := (remember{memory: m}).Execute(context.Background(),
		memoryExecution(rememberArgs("Traditions", "Thursday is game night."))); err != nil {
		t.Fatal(err)
	}

	got := m.Load("424242")
	if !strings.Contains(got, "## Traditions") {
		t.Errorf("no section heading: %q", got)
	}
	if !strings.Contains(got, "Thursday is game night.") {
		t.Errorf("the claim was not recorded: %q", got)
	}
	if !strings.Contains(got, "_(2026-08-04, @drew)_") {
		t.Errorf("the attribution is missing: %q", got)
	}
}

func TestAClaimIsStampedWithWhoSaidItRatherThanWhoWokeBeanbot(t *testing.T) {
	// The whole point. Kate said it on the 1st; drew merely happened to be the one
	// talking when BeanBot noticed, three days later, and vouches for nothing.
	m := openTestMemory(t, 8<<10, nil)

	if _, err := (remember{memory: m}).Execute(context.Background(),
		memoryExecution(cite(map[string]any{
			"section": "Traditions",
			"claim":   "Thursday is game night.",
		}, 1, "kate"))); err != nil {
		t.Fatal(err)
	}

	got := m.Load("424242")
	if !strings.Contains(got, "_(2026-08-01, @kate)_") {
		t.Errorf("the claim was attributed to the trigger rather than its source: %q", got)
	}
	if strings.Contains(got, "@drew") {
		t.Errorf("the member who woke beanbot was named: %q", got)
	}
}

func TestAMiscountedLineIsRefusedRatherThanFiledUnderTheNeighbour(t *testing.T) {
	// Off by one lands on a real message under a real different name, so without
	// the check it files a plausible lie that nothing downstream could ever spot.
	m := openTestMemory(t, 8<<10, nil)

	_, err := (remember{memory: m}).Execute(context.Background(),
		memoryExecution(cite(map[string]any{
			"section": "Traditions",
			"claim":   "Thursday is game night.",
		}, 2, "kate")))

	if err == nil {
		t.Fatal("a line number that disagrees with the name should be refused")
	}
	if got := m.Load("424242"); strings.Contains(got, "game night") {
		t.Errorf("the claim was written anyway: %q", got)
	}
}

func TestALineThatIsNotThereIsRefused(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	for _, line := range []int{0, -1, 4} {
		_, err := (remember{memory: m}).Execute(context.Background(),
			memoryExecution(cite(map[string]any{
				"section": "Traditions",
				"claim":   "Thursday is game night.",
			}, line, "drew")))
		if err == nil {
			t.Errorf("line %d is not in the backlog and should have been refused", line)
		}
	}
}

func TestBeanbotMayNotBeTheSourceOfWhatItAlreadyKnows(t *testing.T) {
	// Otherwise the loop closes: it answers from its notes, then files its own
	// reply as a fresh dated Claim, and a wrong note is now attested twice.
	m := openTestMemory(t, 8<<10, nil)

	_, err := (remember{memory: m}).Execute(context.Background(),
		memoryExecution(cite(map[string]any{
			"section": "Traditions",
			"claim":   "Thursday is game night.",
		}, 3, "BeanBot")))

	if err == nil {
		t.Fatal("beanbot citing its own line should be refused")
	}
}

func TestALineNumberArrivesFromTheModelAsAFloat(t *testing.T) {
	// Function-call arguments are decoded JSON, where every number is a float64.
	// Asserting int finds nothing and a line the model did supply looks absent.
	m := openTestMemory(t, 8<<10, nil)

	args := map[string]any{"section": "Traditions", "claim": "Thursday is game night."}
	args["said_in"], args["said_by"] = float64(1), "kate"

	if _, err := (remember{memory: m}).Execute(context.Background(), memoryExecution(args)); err != nil {
		t.Fatal(err)
	}
	if got := m.Load("424242"); !strings.Contains(got, "_(2026-08-01, @kate)_") {
		t.Errorf("a float line number did not resolve: %q", got)
	}
}

func TestAClaimWithNoSourceIsRefused(t *testing.T) {
	// Falling back to the Trigger would reintroduce the bug intermittently,
	// leaving right and wrong Attributions interleaved with nothing to tell them
	// apart. A Claim occasionally going unwritten is the cheaper failure: BeanBot
	// writes passively and the conversation is still there next turn.
	m := openTestMemory(t, 8<<10, nil)

	for _, args := range []map[string]any{
		{"section": "Traditions", "claim": "Thursday is game night."},
		{"section": "Traditions", "claim": "Thursday is game night.", "said_in": 1},
		{"section": "Traditions", "claim": "Thursday is game night.", "said_by": "kate"},
	} {
		if _, err := (remember{memory: m}).Execute(context.Background(), memoryExecution(args)); err == nil {
			t.Errorf("args %v name no source and should have been refused", args)
		}
	}
}

func TestRememberingCorrectsInsteadOfContradicting(t *testing.T) {
	// There is no forget Capability, so superseding is the only way a wrong
	// note gets fixed before Compaction eventually runs.
	m := openTestMemory(t, 8<<10, nil)
	r := remember{memory: m}

	if _, err := r.Execute(context.Background(),
		memoryExecution(rememberArgs("Traditions", "Steve is allergic to peanuts."))); err != nil {
		t.Fatal(err)
	}

	args := rememberArgs("Traditions", "Steve is not allergic to anything.")
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

	inv := memoryExecution(rememberArgs("Traditions", "Thursday is game night."))
	inv.GuildID = ""

	if _, err := (remember{memory: m}).Execute(context.Background(), inv); err == nil {
		t.Error("a Trigger with no guild has no Memory to write to")
	}
}

func TestRememberingNothingIsRefused(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)
	r := remember{memory: m}

	for _, args := range []map[string]any{
		cite(map[string]any{"claim": "Thursday is game night."}, 2, "drew"),
		cite(map[string]any{"section": "Traditions"}, 2, "drew"),
		cite(map[string]any{"section": "Traditions", "claim": ""}, 2, "drew"),
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
