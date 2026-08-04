package beanbot

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

const botID = "999"

func TestNamingBeanbotTriggersAResponse(t *testing.T) {
	m := &discordgo.Message{Author: &discordgo.User{ID: "1"}, Content: "hey Beanbot how are you"}

	if !isTrigger(m, botID) {
		t.Error("expected a message naming beanbot to trigger a response")
	}
}

func TestMentioningBeanbotTriggersAResponse(t *testing.T) {
	// A real @mention arrives as "<@999>" and never contains the literal
	// string "beanbot", so the substring check alone misses it entirely.
	m := &discordgo.Message{
		Author:   &discordgo.User{ID: "1"},
		Content:  "<@999> you up?",
		Mentions: []*discordgo.User{{ID: botID}},
	}

	if !isTrigger(m, botID) {
		t.Error("expected an @mention of beanbot to trigger a response")
	}
}

func TestReplyingToBeanbotTriggersAResponse(t *testing.T) {
	// Replying is how people continue a conversation; the reply body itself
	// usually names nobody.
	m := &discordgo.Message{
		Author:            &discordgo.User{ID: "1"},
		Content:           "wait really?",
		ReferencedMessage: &discordgo.Message{Author: &discordgo.User{ID: botID}},
	}

	if !isTrigger(m, botID) {
		t.Error("expected a reply to beanbot to trigger a response")
	}
}

func TestReplyingToSomeoneElseDoesNotTrigger(t *testing.T) {
	m := &discordgo.Message{
		Author:            &discordgo.User{ID: "1"},
		Content:           "wait really?",
		ReferencedMessage: &discordgo.Message{Author: &discordgo.User{ID: "2"}},
	}

	if isTrigger(m, botID) {
		t.Error("expected a reply to another member to be ignored")
	}
}

func TestBeanbotDoesNotTriggerOnItsOwnMessages(t *testing.T) {
	// Beanbot's replies routinely contain its own name, so without this guard
	// it would talk to itself forever.
	m := &discordgo.Message{Author: &discordgo.User{ID: botID}, Content: "beanbot out"}

	if isTrigger(m, botID) {
		t.Error("expected beanbot to ignore its own messages")
	}
}
