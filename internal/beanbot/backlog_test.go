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
