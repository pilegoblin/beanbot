package beanbot

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// source is one numbered line of the Backlog as somewhere a Claim may have come
// from, carrying what Discord itself says about who spoke and when.
//
// Capabilities take these rather than *discordgo.Message so that resolving and
// refusing an Attribution needs no network, the same reason Guild is a narrow
// interface. The time is already in the Configured Timezone, so the day a Claim
// is stamped with and the line the model read it off cannot disagree.
type source struct {
	speaker string
	at      time.Time
	// mine marks one of BeanBot's own lines. They are numbered like any other so
	// the numbering matches what the model sees, and refused as a Source: a Claim
	// sourced from BeanBot is BeanBot re-filing its own recollection as testimony.
	mine bool
}

// renderBacklog turns a Backlog into the compact, oldest-first backlog
// BeanBot reads as context. Discord's wire format carries far more structure
// than the model needs, so only the parts that mean something survive: who
// spoke, when, what they said, and what they attached.
//
// Discord returns messages newest-first; the backlog reads oldest-first.
//
// The Sources it returns are the numbered lines it actually emitted, in order,
// so line n is sources[n-1]. They are built here rather than derived from the
// messages afterwards because rendering skips some of them — an authorless
// message, or one whose body comes out empty — and a numbering derived
// separately would drift on exactly the Backlogs with joins and pins in them.
func renderBacklog(backlog []*discordgo.Message, botID string, loc *time.Location) (string, []source) {
	var (
		lines   = make([]string, 0, len(backlog))
		sources = make([]source, 0, len(backlog))
		day     string
	)

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

		when := m.Timestamp.In(loc)
		mine := m.Author.ID == botID
		speaker := displayName(m)
		if mine {
			speaker += " (you)"
		}

		// The date is restated only when it changes. A Backlog of fifty messages
		// may span an afternoon or three months, and a model shown nothing but
		// clock times against a "Current time" of today reads the whole window as
		// happening now — which misjudges how stale a Claim is, and resolves
		// "friday" from last week against this one.
		if today := when.Format("Monday 2 January 2006"); today != day {
			day = today
			lines = append(lines, "— "+today+" —")
		}

		sources = append(sources, source{speaker: displayName(m), at: when, mine: mine})
		lines = append(lines, fmt.Sprintf("%d. [%s] %s: %s",
			len(sources), when.Format("15:04"), speaker, body))
	}

	return strings.Join(lines, "\n"), sources
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
