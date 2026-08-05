package gemini

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEveryVoiceIsNamedAndDescribed(t *testing.T) {
	// The names are documentation-only strings that appear in no SDK constant,
	// so nothing in the type system notices a typo. Characteristics repeat by
	// design — "firm" covers three — because nothing chooses by label; a blank
	// one is still a mistake, since the smoke test logs it.
	for i, v := range Voices {
		if v.Name == "" {
			t.Errorf("voice %d has no name", i)
		}
		if v.Characteristic == "" {
			t.Errorf("%s has no characteristic", v.Name)
		}
	}
}

func TestRandomVoiceOnlyEverPicksARealOne(t *testing.T) {
	// A wrong name does not fail loudly: it reaches Gemini as a rejected
	// request in the middle of a conversation.
	known := map[string]bool{}
	for _, v := range Voices {
		known[v.Name] = true
	}

	seen := map[string]bool{}
	for range 200 {
		got := RandomVoice()
		if !known[got] {
			t.Fatalf("picked %q, which is not in the catalogue", got)
		}
		seen[got] = true
	}

	// Two hundred draws from thirty voices missing more than a few of them
	// would mean the index is not spread across the slice.
	if len(seen) < len(Voices)/2 {
		t.Errorf("200 draws produced only %d of %d voices", len(seen), len(Voices))
	}
}

func TestWavHeaderDescribesThePayloadItCarries(t *testing.T) {
	// Discord plays a WAV and shrugs at headerless PCM, so the 44 bytes in
	// front of the samples are the whole difference between a clip and a blob.
	pcm := bytes.Repeat([]byte{0x01, 0x02}, 100) // 100 frames of 16-bit mono
	wav := wavFromPCM(pcm, 24000)

	if len(wav) != len(pcm)+44 {
		t.Fatalf("wav is %d bytes, want %d", len(wav), len(pcm)+44)
	}
	if got := string(wav[0:4]); got != "RIFF" {
		t.Errorf("chunk id is %q, want RIFF", got)
	}
	if got := string(wav[8:12]); got != "WAVE" {
		t.Errorf("format is %q, want WAVE", got)
	}
	if got := string(wav[12:16]); got != "fmt " {
		t.Errorf("subchunk is %q, want %q", got, "fmt ")
	}
	if got := string(wav[36:40]); got != "data" {
		t.Errorf("data subchunk id is %q, want data", got)
	}

	// A wrong size here is the difference between a clip that plays to the end
	// and one that stops early or hisses past it.
	if got := binary.LittleEndian.Uint32(wav[4:8]); got != uint32(36+len(pcm)) {
		t.Errorf("riff size is %d, want %d", got, 36+len(pcm))
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != uint32(len(pcm)) {
		t.Errorf("data size is %d, want %d", got, len(pcm))
	}
	if !bytes.Equal(wav[44:], pcm) {
		t.Error("the samples were altered on the way into the container")
	}
}

func TestWavHeaderDeclaresMono16BitAtTheGivenRate(t *testing.T) {
	// Getting the rate wrong does not fail — it plays back at the wrong speed,
	// which is why the rate is read from the response rather than assumed.
	wav := wavFromPCM([]byte{0, 0, 0, 0}, 48000)

	if got := binary.LittleEndian.Uint16(wav[20:22]); got != 1 {
		t.Errorf("audio format is %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(wav[22:24]); got != 1 {
		t.Errorf("channel count is %d, want 1 (mono)", got)
	}
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != 48000 {
		t.Errorf("sample rate is %d, want 48000", got)
	}
	if got := binary.LittleEndian.Uint16(wav[34:36]); got != 16 {
		t.Errorf("bits per sample is %d, want 16", got)
	}
	// byteRate = rate * channels * bytesPerSample; blockAlign = channels * bytesPerSample.
	if got := binary.LittleEndian.Uint32(wav[28:32]); got != 48000*2 {
		t.Errorf("byte rate is %d, want %d", got, 48000*2)
	}
	if got := binary.LittleEndian.Uint16(wav[32:34]); got != 2 {
		t.Errorf("block align is %d, want 2", got)
	}
}

func TestSampleRateIsReadFromTheResponseMIME(t *testing.T) {
	// The model states its own rate. Hardcoding 24000 works right up until a
	// model change makes every clip play back as a chipmunk.
	cases := []struct {
		mime string
		want int
	}{
		{"audio/L16;codec=pcm;rate=24000", 24000},
		{"audio/L16;codec=pcm;rate=48000", 48000},
		{"audio/L16; codec=pcm; rate=16000", 16000},
		{"AUDIO/L16;CODEC=PCM;RATE=22050", 22050},
		{"audio/L16;rate=8000;codec=pcm", 8000},
	}

	for _, c := range cases {
		if got := sampleRateFrom(c.mime); got != c.want {
			t.Errorf("sampleRateFrom(%q) = %d, want %d", c.mime, got, c.want)
		}
	}
}

func TestAnUnreadableRateFallsBackRatherThanFailing(t *testing.T) {
	// A clip at a plausible-but-wrong speed beats no clip at all, and 24kHz is
	// what every documented Gemini TTS response has carried.
	cases := []string{"audio/L16;codec=pcm", "", "audio/wav", "audio/L16;rate=banana", "audio/L16;rate="}

	for _, mime := range cases {
		if got := sampleRateFrom(mime); got != defaultSampleRate {
			t.Errorf("sampleRateFrom(%q) = %d, want the %d fallback", mime, got, defaultSampleRate)
		}
	}
}
