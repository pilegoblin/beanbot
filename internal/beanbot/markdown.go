package beanbot

import "strings"

// inlineMarkdown neutralises the characters Discord reads as formatting
// anywhere in a line. The backslash goes first: escaping the others inserts
// backslashes that must not themselves be escaped afterwards.
var inlineMarkdown = strings.NewReplacer(
	`\`, `\\`,
	`*`, `\*`,
	`_`, `\_`,
	`~`, `\~`,
	"`", "\\`",
	`|`, `\|`,
)

// lineMarkdown are the characters that only mean something at the start of a
// line, turning it into a quote, heading or list item.
const lineMarkdown = ">#-"

// escapeMarkdown makes text render in Discord exactly as written. BeanBot
// speaks in emoticons, and Discord reads most of them as markup: "o_O ^_^"
// pairs its underscores and italicises everything between them. The persona
// never wants formatting, so all of it is escaped.
//
// URLs are left intact, since a backslash inside one stops Discord
// auto-linking it.
func escapeMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = escapeLine(line)
	}
	return strings.Join(lines, "\n")
}

func escapeLine(line string) string {
	words := strings.Split(line, " ")
	for i, word := range words {
		if isLink(word) {
			continue
		}
		words[i] = inlineMarkdown.Replace(word)
	}
	escaped := strings.Join(words, " ")

	trimmed := strings.TrimLeft(escaped, " ")
	if trimmed != "" && strings.IndexByte(lineMarkdown, trimmed[0]) >= 0 {
		indent := len(escaped) - len(trimmed)
		return escaped[:indent] + `\` + trimmed
	}
	return escaped
}

func isLink(word string) bool {
	return strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://")
}
