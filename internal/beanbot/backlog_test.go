package beanbot

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func at(hhmm string) time.Time {
	t, err := time.Parse("15:04", hhmm)
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

	got := renderBacklog(backlog, botID, time.UTC)

	want := "[14:03] Drew: anyone up for smash tonight\n[14:04] Sam: i'm down after 8"
	if got != want {
		t.Errorf("backlog out of order:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBacklogResolvesMentionsToNames(t *testing.T) {
	// Raw "<@1>" tells the model nothing about who is being addressed.
	m := msg("Drew", "1", "<@2> and <@999> should settle this", "14:05")
	m.Mentions = []*discordgo.User{
		{ID: "2", Username: "Sam"},
		{ID: botID, Username: "BeanBot"},
	}

	got := renderBacklog([]*discordgo.Message{m}, botID, time.UTC)

	if !strings.Contains(got, "@Sam and @BeanBot should settle this") {
		t.Errorf("expected mentions resolved to names, got:\n%s", got)
	}
}

func TestBacklogLabelsBeanbotsOwnLines(t *testing.T) {
	// Beanbot must be able to tell what it already said from what was said to it.
	backlog := []*discordgo.Message{
		msg("BeanBot", botID, "i have made the event", "14:06"),
	}

	got := renderBacklog(backlog, botID, time.UTC)

	if !strings.HasPrefix(got, "[14:06] BeanBot (you):") {
		t.Errorf("expected beanbot's own line marked as its own, got:\n%s", got)
	}
}

func TestBacklogNotesAttachments(t *testing.T) {
	// The image bytes are not sent with the backlog, so the model needs to know
	// something was there rather than seeing an empty message.
	m := msg("Kim", "3", "look at this", "14:02")
	m.Attachments = []*discordgo.MessageAttachment{{Filename: "cat.png", ContentType: "image/png"}}

	got := renderBacklog([]*discordgo.Message{m}, botID, time.UTC)

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

	got := renderBacklog(backlog, botID, time.UTC)

	if strings.Count(got, "\n") != 0 {
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
