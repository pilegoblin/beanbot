package beanbot

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// isTrigger reports whether a message should wake BeanBot. BeanBot never
// speaks uninvited: it responds only when named, mentioned, or replied to.
func isTrigger(m *discordgo.Message, botID string) bool {
	if m.Author == nil || m.Author.ID == botID {
		return false
	}
	if m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil &&
		m.ReferencedMessage.Author.ID == botID {
		return true
	}
	if strings.Contains(strings.ToLower(m.Content), "beanbot") {
		return true
	}
	for _, u := range m.Mentions {
		if u.ID == botID {
			return true
		}
	}
	return false
}
