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

func speechArgs(words, style string) map[string]any {
	args := map[string]any{"words": words}
	if style != "" {
		args["style"] = style
	}
	return args
}

func TestSpokenWordsReachTheModelAndTheClipReachesTheChannel(t *testing.T) {
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	result, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs("beans are ready", "slow and menacing")))
	if err != nil {
		t.Fatalf("speaking failed: %v", err)
	}

	if speaker.words != "beans are ready" {
		t.Errorf("said %q, want the words it was given", speaker.words)
	}
	if !isRealVoice(speaker.voice) {
		t.Errorf("spoke as %q, which is not a voice Gemini has", speaker.voice)
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
		execution(&fakeGuild{}, speechArgs("hello", ""))); err != nil {
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
		execution(&fakeGuild{}, speechArgs(strings.Repeat("a", maxSpokenChars+1), "")))
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
		execution(&fakeGuild{}, speechArgs(strings.Repeat("a", maxSpokenChars), ""))); err != nil {
		t.Fatalf("text exactly at the cap should be allowed: %v", err)
	}
}

func TestTheCapCountsCharactersNotBytes(t *testing.T) {
	// A cap measured in bytes silently gives a Cyrillic or emoji script half
	// the allowance of an English one, for the same amount of speech.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs(strings.Repeat("я", maxSpokenChars), ""))); err != nil {
		t.Fatalf("multibyte text within the cap should be allowed: %v", err)
	}
}

func TestOverlongStyleIsRefused(t *testing.T) {
	// A stage direction is prefixed onto the script the model reads, so an
	// essay in the style field is another way to buy a long clip.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, speechArgs("hi", strings.Repeat("b", maxStyleChars+1)))); err == nil {
		t.Fatal("expected an overlong style to be refused")
	}
	if speaker.calls != 0 {
		t.Error("the model was called despite the refusal")
	}
}

func TestTheVoiceIsNotTheModelsToChoose(t *testing.T) {
	// BeanBot is a bean computer with no vocal cords, so it has no voice of its
	// own to offer and the model has none to pick. A voice argument reaching the
	// Capability must change nothing about what comes out.
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	declared := generateSpeech{}.Declaration().Parameters.Properties
	if _, ok := declared["voice"]; ok {
		t.Error("the model is still being offered a voice to choose")
	}

	args := speechArgs("hello", "")
	args["voice"] = "Gandalf"
	if _, err := cap.Execute(context.Background(), execution(&fakeGuild{}, args)); err != nil {
		t.Fatalf("an invented voice should be ignored, not refused: %v", err)
	}
	if speaker.voice == "Gandalf" {
		t.Error("the model talked beanbot into a voice that does not exist")
	}
}

func TestTheVoiceVariesBetweenClips(t *testing.T) {
	// The joke is that nobody knows who it will sound like. A constant would
	// pass every other test in this file.
	cap := generateSpeech{maker: &fakeSpeaker{}}
	seen := map[string]bool{}

	for range 50 {
		speaker := &fakeSpeaker{}
		cap = generateSpeech{maker: speaker}
		if _, err := cap.Execute(context.Background(),
			execution(&fakeGuild{}, speechArgs("beans", ""))); err != nil {
			t.Fatalf("speaking failed: %v", err)
		}
		if !isRealVoice(speaker.voice) {
			t.Fatalf("spoke as %q, which is not a voice Gemini has", speaker.voice)
		}
		seen[speaker.voice] = true
	}

	if len(seen) < 2 {
		t.Errorf("fifty clips used %d distinct voice(s); the choice is not random", len(seen))
	}
}

func isRealVoice(name string) bool {
	for _, v := range gemini.Voices {
		if v.Name == name {
			return true
		}
	}
	return false
}

func TestMissingWordsAreRefused(t *testing.T) {
	speaker := &fakeSpeaker{}
	cap := generateSpeech{maker: speaker}

	if _, err := cap.Execute(context.Background(),
		execution(&fakeGuild{}, map[string]any{})); err == nil {
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
		execution(&fakeGuild{}, speechArgs("hello", ""))); err == nil {
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

func TestTheWholeCatalogueIsInPlay(t *testing.T) {
	// The shortlist was curated when something chose from it by label. Nothing
	// does now, so a voice left out is a sound nobody ever hears.
	if len(gemini.Voices) != 30 {
		t.Errorf("the catalogue has %d voices, want all 30", len(gemini.Voices))
	}
}
