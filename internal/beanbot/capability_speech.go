package beanbot

import (
	"bytes"
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
	"google.golang.org/genai"
)

const (
	// maxSpokenChars is roughly eighty seconds of speech — a joke, a threat or
	// an announcement, but not a podcast. It lives in Go rather than in the
	// tool description because Memory is member-writable and injected into
	// every prompt, so a limit the model merely knows about is one it can be
	// talked out of. Eighty seconds is also a ~4MB WAV, comfortably inside
	// Discord's attachment limit.
	maxSpokenChars = 1000
	// maxStyleChars bounds the stage direction. It is prefixed onto the script
	// the model reads aloud, so an essay here is another way to buy a long
	// clip; a real direction is a handful of words.
	maxStyleChars = 200
)

// speechMaker is the part of the model client the speech Capability needs.
type speechMaker interface {
	GenerateSpeech(ctx context.Context, words, voice, style string) (gemini.Clip, error)
}

// generateSpeech reads something aloud and posts the Clip. Like drawing, it
// costs money and changes nothing in the Guild, so it is budgeted by Medium
// rather than by Mutating.
type generateSpeech struct{ maker speechMaker }

func (generateSpeech) RequiredPermission() int64 { return 0 }

func (generateSpeech) Mutating() bool { return false }

func (generateSpeech) Medium() Medium { return MediumClip }

// speechCues are the words that put speech On Offer. Every one of them can
// only mean sound.
//
// "say", "tell" and "announce" are excluded on purpose, and they are the ones
// worth explaining. They are how people ask for an ordinary answer — "beanbot
// tell everyone the plan", "what did dave say" — so admitting them would leave
// speech declared on most Triggers, which is the thing this is for. The cost is
// that "say it louder" gets typed back rather than spoken.
//
// Inflections are listed rather than matched by prefix: prefix matching would
// take "singapore" for "sing" and "spearmint" for "speak". Every phrasing
// somebody actually uses and this list lacks is a code change, which is the
// standing price of deciding this in Go instead of in a prompt.
var speechCues = []string{
	"aloud", "out loud", "read out",
	"speak", "speaks", "speaking", "spoken", "speech",
	"sing", "sings", "singing", "sung",
	"voice", "voices",
	"accent", "accents",
	"impression", "impressions", "impersonate", "impersonation",
	"narrate", "narrates", "narration",
	"whisper", "whispers", "whispering",
	"audio", "tts",
}

func (generateSpeech) Cues() []string { return speechCues }

func (generateSpeech) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: "generate_speech",
		Description: "Say something out loud and post the clip to the channel. Use when " +
			"someone asks you to speak, sing, read something aloud, or do a voice.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"words": {
					Type: genai.TypeString,
					Description: fmt.Sprintf("Exactly the words to say aloud, and nothing else — "+
						"no stage directions, they will be read out. At most %d characters.", maxSpokenChars),
				},
				"style": {
					Type: genai.TypeString,
					Description: "Optional direction for the delivery, as a short phrase completing " +
						"\"say the following, ...\" — for example \"slowly and menacingly\" or " +
						"\"barely holding back laughter\". Omit for a straight reading.",
				},
			},
			Required: []string{"words"},
		},
	}
}

func (g generateSpeech) Execute(ctx context.Context, inv Execution) (Result, error) {
	words, err := argString(inv.Args, "words")
	if err != nil {
		return Result{}, err
	}
	// Counted in characters, not bytes: a byte cap would quietly give a
	// Cyrillic script half the allowance of an English one.
	if n := utf8.RuneCountInString(words); n > maxSpokenChars {
		return Result{}, fmt.Errorf(
			"that is %d characters and i can only say %d at a time — try again with something shorter",
			n, maxSpokenChars)
	}

	style := optionalString(inv.Args, "style")
	if utf8.RuneCountInString(style) > maxStyleChars {
		return Result{}, fmt.Errorf("that direction is too long — keep it under %d characters",
			maxStyleChars)
	}

	// The voice is drawn here rather than asked of the model: BeanBot is a bean
	// computer with no vocal cords to have a voice of its own, so every Clip is
	// borrowed from somebody.
	clip, err := g.maker.GenerateSpeech(ctx, words, gemini.RandomVoice(), style)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Summary: "The clip was made and is being posted to the channel now.",
		Files: []*discordgo.File{{
			Name:        "beanbot.wav",
			ContentType: clip.MIME,
			Reader:      bytes.NewReader(clip.Data),
		}},
	}, nil
}
