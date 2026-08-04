package beanbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

// remember writes something to the Guild's Memory so BeanBot still knows it
// once this conversation has scrolled out of the Backlog.
//
// Ungated: the point is that BeanBot learns from the people in the server, and
// a Memory only moderators may teach is a mod-maintained wiki. Anything a
// member writes here is injected into every later prompt in that Guild, so it
// is rendered as fenced, attributed observations rather than instruction — and
// it cannot reach a Gate, which reads live Discord permissions in Go.
type remember struct{ memory *Memory }

func (remember) RequiredPermission() int64 { return 0 }

// Mutating reports Guild-mutating, and a Memory write is not: it costs no API
// call and changes nothing in Discord. Sharing the single mutation budget with
// create_event would break "make the event and remember we do this weekly",
// which is the composition the turn loop exists for.
func (remember) Mutating() bool { return false }

func (remember) Medium() Medium { return NoMedium }

func (remember) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: "remember",
		Description: "Write something down in your long-term notes about this server, so you " +
			"still know it in conversations weeks from now. Use it for things that last about " +
			"the server itself — running jokes, traditions, how the place works — not for " +
			"passing chatter, and not for anything about a particular person, which goes in " +
			"remember_person instead. Also use it to correct a note that is now wrong.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"section": {
					Type: genai.TypeString,
					Description: "The heading to file this under, e.g. \"Traditions\", " +
						"\"Running jokes\", \"How this server works\". Reuse a heading already in " +
						"your notes whenever one fits. \"People\" is not available here — notes " +
						"about a person go in remember_person.",
				},
				"entry": {
					Type: genai.TypeString,
					Description: "The thing to record, as one self-contained sentence. Write it so " +
						"it still makes sense months from now to someone who cannot see this " +
						"conversation — name people rather than saying \"he\" or \"they\".",
				},
				"replaces": {
					Type: genai.TypeString,
					Description: "Optional. To correct or update something already in your notes, " +
						"quote enough of that existing entry to identify it and it will be " +
						"rewritten in place. Leave this out to record something new.",
				},
			},
			Required: []string{"section", "entry"},
		},
	}
}

func (r remember) Execute(_ context.Context, inv Execution) (Result, error) {
	section, err := argString(inv.Args, "section")
	if err != nil {
		return Result{}, err
	}
	entry, err := argString(inv.Args, "entry")
	if err != nil {
		return Result{}, err
	}

	ch := change{
		Section:  section,
		Entry:    attribute(entry, inv.Author, inv.Now),
		Replaces: optionalString(inv.Args, "replaces"),
	}
	if err := r.memory.Record(inv.GuildID, ch); err != nil {
		return Result{}, err
	}

	return Result{Summary: fmt.Sprintf("Noted under %q: %s", section, entry)}, nil
}

// attribute stamps an entry with when it was recorded and who prompted it.
// Writes are ungated, so attribution is what lets someone reading the file
// later tell a shared fact from one member's mischief.
func attribute(entry, author string, now time.Time) string {
	who := strings.TrimSpace(author)
	if who == "" {
		who = "someone"
	}
	return fmt.Sprintf("%s _(%s, @%s)_", strings.TrimSpace(entry), now.Format("2006-01-02"), who)
}
