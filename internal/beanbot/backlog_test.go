package beanbot

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// at parses a test timestamp, defaulting to 4 August 2026 — a Tuesday — when
// only a clock time is given. Dates are real ones because the Backlog now prints
// the day, so a zero-value year would render as "Monday 1 January 1".
func at(when string) time.Time {
	if t, err := time.Parse("2006-01-02 15:04", when); err == nil {
		return t
	}
	t, err := time.Parse("2006-01-02 15:04", "2026-08-04 "+when)
	if err != nil {
		panic(err)
	}
	return t
}

func msg(author, id, content, when string) *discordgo.Message {
	return &discordgo.Message{
		Author:    &discordgo.User{ID: id, Username: author},
		Content:   content,
		Timestamp: at(when),
	}
}

func TestBacklogReadsOldestFirst(t *testing.T) {
	// Discord returns ChannelMessages newest-first; a backlog that reads
	// backwards would invert every "and then he said" in the conversation.
	backlog := []*discordgo.Message{
		msg("Sam", "2", "i'm down after 8", "14:04"),
		msg("Drew", "1", "anyone up for smash tonight", "14:03"),
	}

	got, _ := renderBacklog(backlog, botID, time.UTC)

	want := "— Tuesday 4 August 2026 —\n" +
		"1. [14:03] Drew: anyone up for smash tonight\n" +
		"2. [14:04] Sam: i'm down after 8"
	if got != want {
		t.Errorf("backlog out of order:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBacklogRestatesTheDateOnlyWhenItChanges(t *testing.T) {
	// A model shown nothing but clock times reads a three-month window as
	// happening this afternoon — it cannot judge how stale a Claim is, and it
	// resolves "friday" from last week against this one.
	backlog := []*discordgo.Message{
		msg("Sam", "2", "sorry, only just saw this", "2026-08-04 09:15"),
		msg("Kim", "3", "maybe", "2026-08-01 14:09"),
		msg("Drew", "1", "anyone up for smash friday", "2026-08-01 14:03"),
	}

	got, _ := renderBacklog(backlog, botID, time.UTC)

	want := "— Saturday 1 August 2026 —\n" +
		"1. [14:03] Drew: anyone up for smash friday\n" +
		"2. [14:09] Kim: maybe\n" +
		"— Tuesday 4 August 2026 —\n" +
		"3. [09:15] Sam: sorry, only just saw this"
	if got != want {
		t.Errorf("day separators wrong:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBacklogNumbersOnlyTheLinesItActuallyPrints(t *testing.T) {
	// The numbering is what a Claim points at, so it has to count printed lines
	// rather than messages. Derived from the message slice instead, it drifts by
	// one on every join and pin — and a drifted number still resolves, to a real
	// neighbouring speaker, which is a misattribution nothing downstream can spot.
	backlog := []*discordgo.Message{
		msg("Kim", "3", "same", "14:05"),
		msg("Sam", "2", "", "14:04"),
		msg("Drew", "1", "smash tonight?", "14:03"),
	}

	got, sources := renderBacklog(backlog, botID, time.UTC)

	if !strings.Contains(got, "1. [14:03] Drew:") || !strings.Contains(got, "2. [14:05] Kim:") {
		t.Errorf("the skipped message left a hole in the numbering:\n%s", got)
	}
	if len(sources) != 2 {
		t.Fatalf("got %d sources for 2 printed lines: %v", len(sources), sources)
	}
	if sources[0].speaker != "Drew" || sources[1].speaker != "Kim" {
		t.Errorf("sources do not line up with the printed lines: %v", sources)
	}
}

func TestBacklogMarksBeanbotsOwnLinesAsItsOwn(t *testing.T) {
	// Numbered like any other line so the numbering matches what the model reads,
	// and flagged so a Claim cannot be sourced from BeanBot's own recollection.
	backlog := []*discordgo.Message{
		msg("BeanBot", botID, "i have made the event", "14:06"),
		msg("Drew", "1", "make an event", "14:05"),
	}

	_, sources := renderBacklog(backlog, botID, time.UTC)

	if len(sources) != 2 {
		t.Fatalf("got %d sources, want 2", len(sources))
	}
	if sources[0].mine {
		t.Error("a member's line was marked as beanbot's own")
	}
	if !sources[1].mine {
		t.Error("beanbot's own line was not marked as its own")
	}
}

func TestBacklogResolvesMentionsToNames(t *testing.T) {
	// Raw "<@1>" tells the model nothing about who is being addressed.
	m := msg("Drew", "1", "<@2> and <@999> should settle this", "14:05")
	m.Mentions = []*discordgo.User{
		{ID: "2", Username: "Sam"},
		{ID: botID, Username: "BeanBot"},
	}

	got, _ := renderBacklog([]*discordgo.Message{m}, botID, time.UTC)

	if !strings.Contains(got, "@Sam and @BeanBot should settle this") {
		t.Errorf("expected mentions resolved to names, got:\n%s", got)
	}
}

func TestBacklogLabelsBeanbotsOwnLines(t *testing.T) {
	// Beanbot must be able to tell what it already said from what was said to it.
	backlog := []*discordgo.Message{
		msg("BeanBot", botID, "i have made the event", "14:06"),
	}

	got, _ := renderBacklog(backlog, botID, time.UTC)

	if !strings.Contains(got, "1. [14:06] BeanBot (you):") {
		t.Errorf("expected beanbot's own line marked as its own, got:\n%s", got)
	}
}

func TestBacklogNotesAttachments(t *testing.T) {
	// The image bytes are not sent with the backlog, so the model needs to know
	// something was there rather than seeing an empty message.
	m := msg("Kim", "3", "look at this", "14:02")
	m.Attachments = []*discordgo.MessageAttachment{{Filename: "cat.png", ContentType: "image/png"}}

	got, _ := renderBacklog([]*discordgo.Message{m}, botID, time.UTC)

	if !strings.Contains(got, "[image: cat.png]") {
		t.Errorf("expected attachment noted inline, got:\n%s", got)
	}
}

func TestBacklogSkipsEmptyMessages(t *testing.T) {
	// Joins, pins and embed-only messages carry no text and would render as
	// content-free noise.
	backlog := []*discordgo.Message{
		msg("Drew", "1", "real message", "14:03"),
		msg("Sam", "2", "", "14:04"),
	}

	got, sources := renderBacklog(backlog, botID, time.UTC)

	if strings.Contains(got, "2.") || len(sources) != 1 {
		t.Errorf("expected empty message dropped, got:\n%s", got)
	}
}

func TestEverySpeakerIsListedOnceWithTheNameTheChannelUses(t *testing.T) {
	// The pairing is what finds a Person whose notes are headed with the name
	// they used to go by, so it has to be the name the Backlog actually printed.
	backlog := []*discordgo.Message{
		msg("Drew", "1", "anyone up for smash tonight", "14:03"),
		msg("Sam", "2", "i'm down after 8", "14:04"),
		msg("Drew", "1", "cool", "14:05"),
	}

	got := speakersIn(backlog)

	want := []namedUser{{ID: "1", Name: "Drew"}, {ID: "2", Name: "Sam"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestAMentionCarriesTheIdentityDiscordResolved(t *testing.T) {
	// The only names besides the author's that a snowflake may be attached to,
	// because they are the only ones Discord stated rather than the model guessed.
	m := msg("Drew", "1", "what do you know about <@2>", "14:03")
	m.Mentions = []*discordgo.User{{ID: "2", Username: "sam", GlobalName: "Sam"}}

	got := mentionsIn(m)

	if len(got) != 1 || got[0] != (namedUser{ID: "2", Name: "Sam"}) {
		t.Errorf("got %v, want one resolved mention for Sam", got)
	}
}
