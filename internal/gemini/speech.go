package gemini

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"google.golang.org/genai"
)

// SpeechModel is the cheapest of Gemini's three text-to-speech models. The
// other two — gemini-3.1-flash-tts-preview and gemini-2.5-pro-preview-tts —
// are both exactly double the price per token, the same way Nano Banana Pro
// is to Nano Banana. It is a preview model, so a withdrawal is a code change
// here rather than a config flip.
const SpeechModel = "gemini-2.5-flash-preview-tts"

// Voice is one of Gemini's prebuilt voices. The name is meaningless astronomy
// and the characteristic is the only thing that says what it sounds like, so
// they always travel together.
type Voice struct {
	Name           string
	Characteristic string
}

// Voices is every prebuilt voice Gemini publishes, and BeanBot speaks in one
// of them at random (ADR 0008).
//
// It is the whole catalogue rather than a shortlist because nothing chooses
// from it by label any more. Six voices were curated when the model picked, and
// the duplicate characteristics were dropped when the list widened, both on the
// same reasoning: the name is meaningless astronomy, so two voices labelled
// "firm" were a choice made blind. A random index does not read the labels, and
// Kore, Orus and Alnilam do not sound alike merely because one word describes
// all three. Every voice dropped for being an indistinguishable *option* is a
// perfectly distinguishable *sound*.
//
// The characteristics stay because the live smoke test logs them, and a human
// listening to thirty clips wants to know which one was supposed to be gravelly.
var Voices = []Voice{
	{"Zephyr", "bright"},
	{"Puck", "upbeat"},
	{"Charon", "informative"},
	{"Kore", "firm"},
	{"Fenrir", "excitable"},
	{"Leda", "youthful"},
	{"Orus", "firm"},
	{"Aoede", "breezy"},
	{"Callirrhoe", "easy-going"},
	{"Autonoe", "bright"},
	{"Enceladus", "breathy"},
	{"Iapetus", "clear"},
	{"Umbriel", "easy-going"},
	{"Algieba", "smooth"},
	{"Despina", "smooth"},
	{"Erinome", "clear"},
	{"Algenib", "gravelly"},
	{"Rasalgethi", "informative"},
	{"Laomedeia", "upbeat"},
	{"Achernar", "soft"},
	{"Alnilam", "firm"},
	{"Schedar", "even"},
	{"Gacrux", "mature"},
	{"Pulcherrima", "forward"},
	{"Achird", "friendly"},
	{"Zubenelgenubi", "casual"},
	{"Vindemiatrix", "gentle"},
	{"Sadachbia", "lively"},
	{"Sadaltager", "knowledgeable"},
	{"Sulafat", "warm"},
}

// RandomVoice picks the voice a Clip is spoken in. BeanBot does not get to
// choose and neither does the model: whoever it sounds like this time is the
// joke.
func RandomVoice() string {
	return Voices[rand.IntN(len(Voices))].Name
}

// Clip is generated speech on its way to a channel, in a container Discord can
// actually play. The model returns headerless PCM; nothing outside this file
// needs to know that.
type Clip struct {
	Data []byte
	MIME string
}

// GenerateSpeech reads words aloud in the named voice. Style is an optional
// direction like "slow and menacing", kept out of the words so that it can
// never be read aloud itself.
func (p *Prompter) GenerateSpeech(ctx context.Context, words, voice, style string) (Clip, error) {
	prompt := words
	if style != "" {
		prompt = fmt.Sprintf("Say the following, %s:\n\n%s", style, words)
	}

	resp, err := p.client.Models.GenerateContent(ctx, SpeechModel,
		[]*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser)},
		&genai.GenerateContentConfig{
			ResponseModalities: []string{"AUDIO"},
			SpeechConfig: &genai.SpeechConfig{
				VoiceConfig: &genai.VoiceConfig{
					PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{VoiceName: voice},
				},
			},
			SafetySettings: permissiveSafety(),
		})
	if err != nil {
		return Clip{}, err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return Clip{}, errors.New("the speech model returned nothing")
	}

	for _, part := range resp.Candidates[0].Content.Parts {
		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			return Clip{
				Data: wavFromPCM(part.InlineData.Data, sampleRateFrom(part.InlineData.MIMEType)),
				MIME: "audio/wav",
			}, nil
		}
	}

	// The model answered in words instead of speech, usually to refuse.
	var refusal strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" && !part.Thought {
			refusal.WriteString(part.Text)
		}
	}
	if refusal.Len() > 0 {
		return Clip{}, fmt.Errorf("no speech was produced: %s", refusal.String())
	}
	return Clip{}, errors.New("no speech was produced")
}

// Gemini returns signed 16-bit little-endian mono samples. Only the rate is
// ever in doubt, and it is stated in the response.
const (
	defaultSampleRate = 24000
	bitsPerSample     = 16
	channels          = 1
	wavHeaderBytes    = 44
)

// sampleRateFrom reads the rate out of a MIME type like
// "audio/L16;codec=pcm;rate=24000". Hardcoding the rate works right up until a
// model change alters it, at which point every clip silently plays back at the
// wrong speed — a failure with no error attached to it. Anything unreadable
// falls back rather than failing: a clip at a plausible speed beats no clip.
func sampleRateFrom(mime string) int {
	for _, param := range strings.Split(mime, ";") {
		key, value, found := strings.Cut(param, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "rate") {
			continue
		}
		if rate, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && rate > 0 {
			return rate
		}
	}
	return defaultSampleRate
}

// wavFromPCM prepends the 44-byte canonical RIFF header that turns raw samples
// into a file Discord will render a player for.
func wavFromPCM(pcm []byte, rate int) []byte {
	const bytesPerSample = bitsPerSample / 8
	blockAlign := channels * bytesPerSample
	byteRate := rate * blockAlign

	wav := make([]byte, wavHeaderBytes, wavHeaderBytes+len(pcm))
	copy(wav[0:4], "RIFF")
	// Everything after this field: the 36 remaining header bytes plus samples.
	binary.LittleEndian.PutUint32(wav[4:8], uint32(36+len(pcm)))
	copy(wav[8:12], "WAVE")

	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16) // fmt chunk length, 16 for PCM
	binary.LittleEndian.PutUint16(wav[20:22], 1)  // 1 is uncompressed PCM
	binary.LittleEndian.PutUint16(wav[22:24], channels)
	binary.LittleEndian.PutUint32(wav[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(wav[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(wav[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(wav[34:36], bitsPerSample)

	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(pcm)))

	return append(wav, pcm...)
}
