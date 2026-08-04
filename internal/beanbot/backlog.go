package beanbot

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// renderBacklog turns a Backlog into the compact, oldest-first backlog
// BeanBot reads as context. Discord's wire format carries far more structure
// than the model needs, so only the parts that mean something survive: who
// spoke, when, what they said, and what they attached.
//
// Discord returns messages newest-first; the backlog reads oldest-first.
func renderBacklog(backlog []*discordgo.Message, botID string, loc *time.Location) string {
	lines := make([]string, 0, len(backlog))

	for i := len(backlog) - 1; i >= 0; i-- {
		m := backlog[i]
		if m.Author == nil {
			continue
		}

		body := resolveMentions(m)
		for _, a := range m.Attachments {
			body = strings.TrimSpace(fmt.Sprintf("[%s: %s] %s", attachmentKind(a), a.Filename, body))
		}
		if body == "" {
			continue
		}

		speaker := displayName(m)
		if m.Author.ID == botID {
			speaker += " (you)"
		}

		lines = append(lines, fmt.Sprintf("[%s] %s: %s",
			m.Timestamp.In(loc).Format("15:04"), speaker, body))
	}

	return strings.Join(lines, "\n")
}

// speakersIn lists everyone who spoke in the Backlog, paired with the name it
// shows them under. That pairing is what finds a Person whose notes are headed
// with the name they used to go by.
func speakersIn(backlog []*discordgo.Message) []namedUser {
	var speakers []namedUser
	seen := map[string]bool{}

	for _, m := range backlog {
		if m == nil || m.Author == nil || seen[m.Author.ID] {
			continue
		}
		seen[m.Author.ID] = true
		speakers = append(speakers, namedUser{ID: m.Author.ID, Name: displayName(m)})
	}
	return speakers
}

// mentionsIn lists the users @-mentioned in one message, already resolved by
// Discord. They are named the way resolveMentions writes them into the Backlog,
// which is the spelling the model will be working from.
func mentionsIn(m *discordgo.Message) []namedUser {
	var mentions []namedUser
	for _, u := range m.Mentions {
		name := u.GlobalName
		if name == "" {
			name = u.Username
		}
		mentions = append(mentions, namedUser{ID: u.ID, Name: name})
	}
	return mentions
}

// resolveMentions rewrites "<@id>" into "@Name" so the model can tell who is
// being addressed. Unresolvable IDs are left alone rather than guessed at.
func resolveMentions(m *discordgo.Message) string {
	content := m.Content
	for _, u := range m.Mentions {
		name := u.GlobalName
		if name == "" {
			name = u.Username
		}
		content = strings.NewReplacer(
			"<@"+u.ID+">", "@"+name,
			"<@!"+u.ID+">", "@"+name,
		).Replace(content)
	}
	return content
}

// displayName prefers the per-guild nickname, since that is what members
// actually call each other in the channel.
func displayName(m *discordgo.Message) string {
	if m.Member != nil && m.Member.Nick != "" {
		return m.Member.Nick
	}
	if m.Author.GlobalName != "" {
		return m.Author.GlobalName
	}
	return m.Author.Username
}

func attachmentKind(a *discordgo.MessageAttachment) string {
	if strings.HasPrefix(strings.ToLower(a.ContentType), "image") {
		return "image"
	}
	return "file"
}
