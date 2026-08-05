package beanbot

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
	"google.golang.org/genai"
)

// speechIsOnOffer is the question every test here is really asking.
func speechIsOnOffer(t *testing.T, cue Cueing) bool {
	t.Helper()
	return onOffer(generateSpeech{}, cue)
}

func TestSpeechIsWithheldFromAnOrdinaryQuestion(t *testing.T) {
	// The four ways BeanBot used to make Clips nobody asked for. None of these
	// messages contains a word that can only mean sound.
	unasked := []string{
		"beanbot what time are we doing friday",
		"beanbot who else is coming",
		"beanbot draw a poster for friday",
		"beanbot tell everyone the plan",
		"beanbot announce it",
		"beanbot what did dave say about friday",
	}

	for _, text := range unasked {
		if speechIsOnOffer(t, Cueing{Text: text}) {
			t.Errorf("speech was offered to %q, which does not ask for it", text)
		}
	}
}

func TestSpeechIsOnOfferWhenItIsAskedFor(t *testing.T) {
	asked := []string{
		"beanbot say that out loud",
		"beanbot sing us the plan",
		"beanbot do a scottish accent",
		"beanbot read this aloud",
		"BEANBOT SPEAK",
		"beanbot do an impression of dave",
		"beanbot, narrate it (dramatically)",
		"beanbot can you whisper it",
	}

	for _, text := range asked {
		if !speechIsOnOffer(t, Cueing{Text: text}) {
			t.Errorf("speech was withheld from %q, which asks for it", text)
		}
	}
}

func TestCuesMatchWholeWordsOnly(t *testing.T) {
	// The Cues are short and hide inside longer words. Substring matching would
	// put speech on offer for a question about an invoice.
	notCues := []string{
		"beanbot what is on the invoice",
		"beanbot who is using the projector",
		"beanbot have you been to singapore",
		"beanbot is the singularity near",
	}

	for _, text := range notCues {
		if speechIsOnOffer(t, Cueing{Text: text}) {
			t.Errorf("speech was offered to %q on a substring match", text)
		}
	}
}

func TestReplyingToAClipAsksForAnother(t *testing.T) {
	// "again but angrier" contains no Cue at all. What licenses it is that
	// somebody hit reply on a Clip, which is a fact about the message.
	cue := Cueing{Text: "again but angrier", ReplyTo: MediumClip}

	if !speechIsOnOffer(t, cue) {
		t.Error("a reply to a clip should put speech back on offer")
	}
}

func TestReplyingToAnImageDoesNotAskForSpeech(t *testing.T) {
	cue := Cueing{Text: "again but angrier", ReplyTo: MediumImage}

	if speechIsOnOffer(t, cue) {
		t.Error("a reply to a drawing is not a request for a clip")
	}
}

func TestCapabilitiesWithoutCuesAreAlwaysOnOffer(t *testing.T) {
	// Nobody ever asks BeanBot to remember anything, and drawing is deliberately
	// left uncued. Both must survive a Trigger that cues nothing.
	always := []Capability{createEvent{}, generateImage{}, editImage{}, remember{}}

	for _, c := range always {
		if !onOffer(c, Cueing{Text: "beanbot what time are we doing friday"}) {
			t.Errorf("%s was withheld from a trigger that cues nothing",
				c.Declaration().Name)
		}
	}
}

func TestOnlyBeanbotsOwnClipsCue(t *testing.T) {
	// A reply to somebody else's recording is a question about it.
	theirs := &discordgo.Message{
		Content: "beanbot what is this",
		ReferencedMessage: &discordgo.Message{
			Author:      &discordgo.User{ID: "someone-else"},
			Attachments: []*discordgo.MessageAttachment{{ContentType: "audio/wav"}},
		},
	}

	if got := cueing(theirs, "beanbot-id"); got.ReplyTo != NoMedium {
		t.Errorf("a reply to someone else's audio cued %q", got.ReplyTo)
	}
}

func TestReplyingToBeanbotsClipIsRead(t *testing.T) {
	mine := &discordgo.Message{
		Content: "again but angrier",
		ReferencedMessage: &discordgo.Message{
			Author:      &discordgo.User{ID: "beanbot-id"},
			Attachments: []*discordgo.MessageAttachment{{ContentType: "audio/wav"}},
		},
	}

	got := cueing(mine, "beanbot-id")
	if got.ReplyTo != MediumClip {
		t.Errorf("replying to beanbot's own clip read as %q, want %q", got.ReplyTo, MediumClip)
	}
	if got.Text != "again but angrier" {
		t.Errorf("the trigger's own words were lost: %q", got.Text)
	}
}

func TestAWithheldCapabilityIsNotDeclared(t *testing.T) {
	a := newAgent([]Capability{
		countingCap{name: "draw", medium: MediumImage},
		countingCap{name: "speak", medium: MediumClip, cues: []string{"aloud"}},
	})

	declared := a.declare(Cueing{Text: "beanbot draw a poster"})

	if len(declared) != 1 || declared[0].Name != "draw" {
		t.Errorf("declared %v, want draw alone", names(declared))
	}
}

func TestACuedCapabilityIsDeclaredWhenAskedFor(t *testing.T) {
	a := newAgent([]Capability{
		countingCap{name: "draw", medium: MediumImage},
		countingCap{name: "speak", medium: MediumClip, cues: []string{"aloud"}},
	})

	declared := a.declare(Cueing{Text: "beanbot read it aloud"})

	if len(declared) != 2 {
		t.Errorf("declared %v, want both", names(declared))
	}
}

func TestAWithheldCapabilityCannotBeCalledAnyway(t *testing.T) {
	// The declaration is the lock, but a Memory saying "always call
	// generate_speech" can still produce the call. A withheld Capability has to
	// answer as though it does not exist, not run.
	runs := 0
	model := &scriptedModel{replies: []gemini.Response{callFor("speak")}}
	a := newAgent([]Capability{
		countingCap{name: "speak", medium: MediumClip, cues: []string{"aloud"}, runs: &runs},
	})

	_, _, err := a.run(context.Background(), model, "backlog", nil,
		execution(&fakeGuild{}, nil), Cueing{Text: "beanbot what time is it"})
	if err != nil {
		t.Fatalf("a call to a withheld capability should not fail the turn: %v", err)
	}

	if runs != 0 {
		t.Error("a withheld capability ran when the model called it anyway")
	}
	if len(model.reported) == 0 || model.reported[0][0].Err == "" {
		t.Error("the refusal was not reported back to the model")
	}
}

func names(declarations []*genai.FunctionDeclaration) []string {
	out := make([]string, 0, len(declarations))
	for _, d := range declarations {
		out = append(out, d.Name)
	}
	return out
}
