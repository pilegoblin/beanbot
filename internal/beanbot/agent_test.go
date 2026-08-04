package beanbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
	"google.golang.org/genai"
)

// scriptedModel replays a fixed sequence of model responses so the loop's
// bounds can be exercised without calling Gemini.
type scriptedModel struct {
	replies []gemini.Response
	calls   int
	// reported records what the loop fed back to the model each round.
	reported [][]gemini.FunctionResult
}

func (s *scriptedModel) next() (gemini.Response, error) {
	if s.calls >= len(s.replies) {
		return gemini.Response{Text: "done"}, nil
	}
	r := s.replies[s.calls]
	s.calls++
	return r, nil
}

func (s *scriptedModel) Ask(context.Context, string, []gemini.Image) (gemini.Response, error) {
	return s.next()
}

func (s *scriptedModel) Report(_ context.Context, results []gemini.FunctionResult) (gemini.Response, error) {
	s.reported = append(s.reported, results)
	return s.next()
}

// countingCap records how often it ran.
type countingCap struct {
	name     string
	mutating bool
	medium   Medium
	perm     int64
	runs     *int
}

func (c countingCap) RequiredPermission() int64 { return c.perm }
func (c countingCap) Mutating() bool            { return c.mutating }
func (c countingCap) Medium() Medium            { return c.medium }
func (c countingCap) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{Name: c.name}
}
func (c countingCap) Execute(context.Context, Execution) (Result, error) {
	*c.runs++
	return Result{Summary: "ok"}, nil
}

func callFor(name string) gemini.Response {
	return gemini.Response{FunctionCalls: []gemini.FunctionCall{{Name: name, Args: map[string]any{}}}}
}

func TestLoopStopsCallingAfterTheIterationCap(t *testing.T) {
	// A model that never stops asking must not be allowed to spend forever.
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{
		callFor("spin"), callFor("spin"), callFor("spin"),
		callFor("spin"), callFor("spin"), callFor("spin"),
		callFor("spin"), callFor("spin"),
	}}
	a := newAgent([]Capability{countingCap{name: "spin", runs: &runs}})

	_, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil))
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if runs > maxIterations {
		t.Errorf("capability ran %d times, cap is %d", runs, maxIterations)
	}
}

func TestModelRoundTripsAreCappedInTotal(t *testing.T) {
	// The opening Ask counts against the budget too; otherwise the real
	// ceiling is maxIterations+1.
	model := &scriptedModel{replies: []gemini.Response{
		callFor("spin"), callFor("spin"), callFor("spin"),
		callFor("spin"), callFor("spin"), callFor("spin"),
		callFor("spin"), callFor("spin"),
	}}
	runs := 0
	a := newAgent([]Capability{countingCap{name: "spin", runs: &runs}})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if model.calls > maxIterations {
		t.Errorf("made %d model round-trips, cap is %d", model.calls, maxIterations)
	}
}

func TestBeanbotStillSpeaksWhenItRunsOutOfRoundTrips(t *testing.T) {
	// Hitting the ceiling leaves a response full of tool calls and no prose.
	// Falling mute is the one outcome the design rules out.
	model := &scriptedModel{replies: []gemini.Response{
		callFor("spin"), callFor("spin"), callFor("spin"),
		callFor("spin"), callFor("spin"), callFor("spin"),
	}}
	runs := 0
	a := newAgent([]Capability{countingCap{name: "spin", runs: &runs}})

	reply, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil))
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if strings.TrimSpace(reply) == "" {
		t.Error("beanbot fell silent after exhausting its round-trips")
	}
}

func TestAFailedMutationDoesNotSpendTheBudget(t *testing.T) {
	// A create_event refused for a bad timestamp must leave room for the model
	// to retry with a corrected one in the same turn.
	cap := &flakyCap{failFirst: true}
	model := &scriptedModel{replies: []gemini.Response{callFor("mutate"), callFor("mutate")}}
	a := newAgent([]Capability{cap})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cap.succeeded != 1 {
		t.Errorf("the corrected retry never ran: %d successes", cap.succeeded)
	}
}

// flakyCap fails its first call and succeeds thereafter.
type flakyCap struct {
	failFirst bool
	attempts  int
	succeeded int
}

func (*flakyCap) RequiredPermission() int64 { return 0 }
func (*flakyCap) Mutating() bool            { return true }
func (*flakyCap) Medium() Medium            { return NoMedium }
func (*flakyCap) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{Name: "mutate"}
}
func (f *flakyCap) Execute(context.Context, Execution) (Result, error) {
	f.attempts++
	if f.failFirst && f.attempts == 1 {
		return Result{}, errors.New("that timestamp is not RFC3339")
	}
	f.succeeded++
	return Result{Summary: "ok"}, nil
}

func TestOnlyOneGuildMutatingCapabilityRunsPerTurn(t *testing.T) {
	// "beanbot make 40 events" is one sentence.
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{
		callFor("mutate"), callFor("mutate"), callFor("mutate"),
	}}
	a := newAgent([]Capability{countingCap{name: "mutate", mutating: true, runs: &runs}})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if runs != 1 {
		t.Errorf("mutating capability ran %d times, want exactly 1", runs)
	}
}

func TestNonMutatingCapabilitiesMayChain(t *testing.T) {
	// Chaining is the point: "draw a poster, then make the event".
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{callFor("read"), callFor("read")}}
	a := newAgent([]Capability{countingCap{name: "read", runs: &runs}})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if runs != 2 {
		t.Errorf("non-mutating capability ran %d times, want 2", runs)
	}
}

func TestOnlyOneCapabilityPerMediumRunsPerTurn(t *testing.T) {
	// A Memory is member-writable and injected into every prompt, so "always
	// draw six pictures" is a durable instruction anyone can plant. The budget
	// is what makes planting it pointless.
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{
		callFor("draw"), callFor("draw"), callFor("draw"),
	}}
	a := newAgent([]Capability{countingCap{name: "draw", medium: MediumImage, runs: &runs}})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if runs != 1 {
		t.Errorf("the image capability ran %d times, want exactly 1", runs)
	}
}

func TestEachMediumHasItsOwnBudget(t *testing.T) {
	// "Draw the poster and read it out" is one request, and the two media cost
	// different amounts at different models. One shared counter would make the
	// cheap one compete with the expensive one for no reason.
	images, clips := 0, 0
	model := &scriptedModel{replies: []gemini.Response{callFor("draw"), callFor("speak")}}
	a := newAgent([]Capability{
		countingCap{name: "draw", medium: MediumImage, runs: &images},
		countingCap{name: "speak", medium: MediumClip, runs: &clips},
	})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if images != 1 || clips != 1 {
		t.Errorf("drew %d and spoke %d, want one of each", images, clips)
	}
}

func TestTheMediumBudgetIsSeparateFromTheMutationBudget(t *testing.T) {
	// "Make the event and say it out loud" has to keep working: a Clip changes
	// nothing in the Guild, so it must not consume the one mutation allowed.
	events, clips := 0, 0
	model := &scriptedModel{replies: []gemini.Response{callFor("event"), callFor("speak")}}
	a := newAgent([]Capability{
		countingCap{name: "event", mutating: true, runs: &events},
		countingCap{name: "speak", medium: MediumClip, runs: &clips},
	})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if events != 1 || clips != 1 {
		t.Errorf("made %d events and %d clips, want one of each", events, clips)
	}
}

func TestAFailedMediaCallDoesNotSpendTheBudget(t *testing.T) {
	// Same rule as a refused mutation: a call rejected for a bad argument must
	// leave room for the model to retry with a corrected one in the same turn.
	cap := &flakyMediaCap{}
	model := &scriptedModel{replies: []gemini.Response{callFor("speak"), callFor("speak")}}
	a := newAgent([]Capability{cap})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cap.succeeded != 1 {
		t.Errorf("the corrected retry never ran: %d successes", cap.succeeded)
	}
}

// flakyMediaCap fails its first call and succeeds thereafter.
type flakyMediaCap struct {
	attempts  int
	succeeded int
}

func (*flakyMediaCap) RequiredPermission() int64 { return 0 }
func (*flakyMediaCap) Mutating() bool            { return false }
func (*flakyMediaCap) Medium() Medium            { return MediumClip }
func (*flakyMediaCap) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{Name: "speak"}
}
func (f *flakyMediaCap) Execute(context.Context, Execution) (Result, error) {
	f.attempts++
	if f.attempts == 1 {
		return Result{}, errors.New("that is longer than i can say in one go")
	}
	f.succeeded++
	return Result{Summary: "ok"}, nil
}

func TestExceedingTheMediumBudgetIsExplainedToTheModel(t *testing.T) {
	// The refusal is the model's only clue about why nothing came back, and it
	// has to name the medium so BeanBot can explain itself in character.
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{callFor("speak"), callFor("speak")}}
	a := newAgent([]Capability{countingCap{name: "speak", medium: MediumClip, runs: &runs}})

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(model.reported) < 2 {
		t.Fatalf("expected two rounds of results, got %d", len(model.reported))
	}
	refusal := model.reported[1][0].Err
	if !strings.Contains(refusal, string(MediumClip)) {
		t.Errorf("refusal %q should name the medium it ran out of", refusal)
	}
}

func TestRefusedCapabilityIsReportedToTheModelNotRaised(t *testing.T) {
	// Beanbot should explain the refusal in character, so the failure has to
	// come back as a tool result rather than aborting the turn.
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{callFor("privileged")}}
	a := newAgent([]Capability{
		countingCap{name: "privileged", perm: discordgo.PermissionManageEvents, runs: &runs},
	})

	_, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{perms: 0}, nil))
	if err != nil {
		t.Fatalf("a refusal should not fail the turn: %v", err)
	}

	if runs != 0 {
		t.Error("the gated capability should never have executed")
	}
	if len(model.reported) == 0 || model.reported[0][0].Err == "" {
		t.Fatal("the refusal was not reported back to the model")
	}
	if !strings.Contains(model.reported[0][0].Err, "permission") {
		t.Errorf("refusal should say why, got %q", model.reported[0][0].Err)
	}
}

func TestUnknownCapabilityIsReportedNotFatal(t *testing.T) {
	model := &scriptedModel{replies: []gemini.Response{callFor("nonexistent")}}
	a := newAgent(nil)

	if _, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil)); err != nil {
		t.Fatalf("a hallucinated tool name should not fail the turn: %v", err)
	}

	if len(model.reported) == 0 || model.reported[0][0].Err == "" {
		t.Error("the unknown tool should have been reported back to the model")
	}
}

func TestFilesFromCapabilitiesReachTheChannel(t *testing.T) {
	model := &scriptedModel{replies: []gemini.Response{callFor("draw")}}
	a := newAgent([]Capability{fileCap{}})

	_, files, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil))
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected the drawn file to be carried out of the loop, got %d", len(files))
	}
}

type fileCap struct{}

func (fileCap) RequiredPermission() int64 { return 0 }
func (fileCap) Mutating() bool            { return false }
func (fileCap) Medium() Medium            { return MediumImage }
func (fileCap) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{Name: "draw"}
}
func (fileCap) Execute(context.Context, Execution) (Result, error) {
	return Result{Summary: "drew", Files: []*discordgo.File{{Name: "x.png"}}}, nil
}
