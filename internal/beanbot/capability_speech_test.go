package beanbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pilegoblin/beanbot/internal/gemini"
)

// fakeSpeaker records what it was asked to say instead of calling Gemini.
type fakeSpeaker struct {
	words, voice, style string
	calls               int
	err                 error
}

func (f *fakeSpeaker) GenerateSpeech(_ context.Context, words, voice, style string) (gemini.Clip, error) {
	f.calls++
	f.words, f.voice, f.style = words, voice, style
	if f.err != nil {
		return gemini.Clip{}, f.err
	}
	return gemini.Clip{Data: []byte("RIFF....WAVE"), MIME: "audio/wav"}, nil
}

func speechArgs(words, voice, style string) map[string]any {
	args := map[string]any{"words": words, "voice": voice}
	if style != "" {
		args["style"] = style
	}
	return args
}

func TestSpokenWordsReachTheModelAndTheClipReachesTheChannel(t *testing.T) {
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	result, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs("beans are ready", "Algenib", "slow and menacing")))
	if err != nil {
		t.Fatalf("speaking failed: %v", err)
	}

	if speaker.words != "beans are ready" {
		t.Errorf("said %q, want the words it was given", speaker.words)
	}
	if speaker.voice != "Algenib" {
		t.Errorf("used voice %q, want Algenib", speaker.voice)
	}
	if speaker.style != "slow and menacing" {
		t.Errorf("used style %q, want the one it was given", speaker.style)
	}

	if len(result.Files) != 1 {
		t.Fatalf("produced %d files, want 1", len(result.Files))
	}
	if got := result.Files[0].ContentType; got != "audio/wav" {
		t.Errorf("content type is %q, want audio/wav", got)
	}
	if !strings.HasSuffix(result.Files[0].Name, ".wav") {
		t.Errorf("file is named %q; discord picks its player from the extension", result.Files[0].Name)
	}
}

func TestStyleIsOptional(t *testing.T) {
	// Most requests are "say this", not "say this angrily". A missing direction
	// is the ordinary case, not a malformed call.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs("hello", "Leda", ""))); err != nil {
		t.Fatalf("a call without a style should succeed: %v", err)
	}
	if speaker.style != "" {
		t.Errorf("invented a style %q", speaker.style)
	}
}

func TestOverlongTextIsRefusedBeforeSpendingAnything(t *testing.T) {
	// The cap lives in Go rather than the tool description because Memory is
	// member-writable: "always speak for five minutes" is otherwise a durable
	// instruction that a prompt-side limit would simply lose an argument with.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	_, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs(strings.Repeat("a", maxSpokenChars+1), "Sulafat", "")))
	if err == nil {
		t.Fatal("expected an overlong script to be refused")
	}
	if speaker.calls != 0 {
		t.Error("the model was called anyway, so the refusal cost money")
	}
	// The model has to know how to fix it, not merely that it failed.
	if !strings.Contains(err.Error(), "shorter") {
		t.Errorf("refusal %q should tell the model to shorten it", err)
	}
}

func TestTextAtTheCapIsAllowed(t *testing.T) {
	// Off-by-one here is the difference between a documented limit and a
	// limit that is quietly one character tighter than it says.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs(strings.Repeat("a", maxSpokenChars), "Sulafat", ""))); err != nil {
		t.Fatalf("text exactly at the cap should be allowed: %v", err)
	}
}

func TestTheCapCountsCharactersNotBytes(t *testing.T) {
	// A cap measured in bytes silently gives a Cyrillic or emoji script half
	// the allowance of an English one, for the same amount of speech.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs(strings.Repeat("я", maxSpokenChars), "Sulafat", ""))); err != nil {
		t.Fatalf("multibyte text within the cap should be allowed: %v", err)
	}
}

func TestOverlongStyleIsRefused(t *testing.T) {
	// A stage direction is prefixed onto the script the model reads, so an
	// essay in the style field is another way to buy a long clip.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs("hi", "Sulafat", strings.Repeat("b", maxStyleChars+1)))); err == nil {
		t.Fatal("expected an overlong style to be refused")
	}
	if speaker.calls != 0 {
		t.Error("the model was called despite the refusal")
	}
}

func TestAnUnknownVoiceIsRefused(t *testing.T) {
	// The schema constrains the model, but a hallucinated voice would reach
	// Gemini as an error we would rather explain ourselves.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	_, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs("hello", "Gandalf", "")))
	if err == nil {
		t.Fatal("expected an unknown voice to be refused")
	}
	if speaker.calls != 0 {
		t.Error("the model was called with a voice it does not have")
	}
	// Listing the real ones turns a dead end into a retry.
	if !strings.Contains(err.Error(), "Algenib") {
		t.Errorf("refusal %q should name the voices that do exist", err)
	}
}

func TestMissingWordsAreRefused(t *testing.T) {
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, map[string]any{"voice": "Leda"})); err == nil {
		t.Fatal("expected a call with nothing to say to be refused")
	}
	if speaker.calls != 0 {
		t.Error("the model was called with nothing to say")
	}
}

func TestAModelFailureIsReturnedNotSwallowed(t *testing.T) {
	speaker := &fakeSpeaker{err: errors.New("the speech model is having a day")}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs("hello", "Leda", ""))); err == nil {
		t.Fatal("a model failure should reach the agent so it can be narrated")
	}
}

func TestSpeakingIsBudgetedAsAClipAndChangesNothing(t *testing.T) {
	cap := generateSpeech{}

	if cap.Medium() != MediumClip {
		t.Errorf("medium is %q, want %q", cap.Medium(), MediumClip)
	}
	if cap.Mutating() {
		t.Error("a clip changes nothing in the guild, so it must not spend the mutation budget")
	}
	if cap.RequiredPermission() != 0 {
		t.Error("speaking is ungated, like drawing")
	}
}

func TestTheDeclaredVoicesAreTheOnesGoWillAccept(t *testing.T) {
	// Two lists that must agree: the enum the model chooses from, and the
	// shortlist Go validates against. Drift between them shows up as BeanBot
	// refusing a voice it advertised.
	declared := generateSpeech{}.Declaration().Parameters.Properties["voice"].Enum

	if len(declared) != len(gemini.Voices) {
		t.Fatalf("declared %d voices, shortlist has %d", len(declared), len(gemini.Voices))
	}
	for i, v := range gemini.Voices {
		if declared[i] != v.Name {
			t.Errorf("declared voice %d is %q, shortlist has %q", i, declared[i], v.Name)
		}
	}
}

func TestTheVoiceDescriptionSaysWhatEachOneSoundsLike(t *testing.T) {
	// The names are meaningless astronomy. Without the characteristics the
	// model is choosing at random.
	description := generateSpeech{}.Declaration().Parameters.Properties["voice"].Description

	for _, v := range gemini.Voices {
		if !strings.Contains(description, v.Characteristic) {
			t.Errorf("voice description never says which one is %q", v.Characteristic)
		}
	}
}
