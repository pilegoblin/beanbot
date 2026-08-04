package beanbot

import (
	"context"
	"fmt"
	"strings"

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
				"claim": {
					Type: genai.TypeString,
					Description: "The thing to record, as one self-contained sentence. Write it so " +
						"it still makes sense months from now to someone who cannot see this " +
						"conversation — name people rather than saying \"he\" or \"they\".",
				},
				"said_in": {
					Type: genai.TypeInteger,
					Description: "The number printed at the start of the line where this was said. " +
						"Your notes record who told you a thing and the day they told you, so this " +
						"must be the line it was actually said on, not the line you are replying " +
						"to. You cannot cite one of your own lines.",
				},
				"said_by": {
					Type: genai.TypeString,
					Description: "The name shown on that line, copied exactly. It is checked against " +
						"the line number, so a miscount is caught rather than filed under whoever " +
						"happened to speak next.",
				},
				"replaces": {
					Type: genai.TypeString,
					Description: "Optional. To correct or update something already in your notes, " +
						"quote enough of that existing entry to identify it and it will be " +
						"rewritten in place. Leave this out to record something new.",
				},
			},
			Required: []string{"section", "claim", "said_in", "said_by"},
		},
	}
}

func (r remember) Execute(_ context.Context, inv Execution) (Result, error) {
	section, err := argString(inv.Args, "section")
	if err != nil {
		return Result{}, err
	}
	claim, err := argString(inv.Args, "claim")
	if err != nil {
		return Result{}, err
	}
	src, err := citation(inv)
	if err != nil {
		return Result{}, err
	}

	ch := change{
		Section:  section,
		Claim:    attribute(claim, src),
		Replaces: optionalString(inv.Args, "replaces"),
	}
	if err := r.memory.Record(inv.GuildID, ch); err != nil {
		return Result{}, err
	}

	return Result{Summary: fmt.Sprintf("Noted under %q: %s", section, claim)}, nil
}

// citation resolves the Source the model named into the Backlog line Go
// rendered, or refuses in terms the model can act on and retry from.
//
// The model points and Go decides. The name it returns is checked against the
// line and then thrown away: what reaches the file is Discord's own record of
// who spoke, so no member can type a name into somebody else's mouth. That check
// is also what makes a miscount fail closed — an ordinal off by one lands on a
// real neighbouring message under a real different name, and would otherwise
// file a plausible lie that nothing downstream could ever spot.
func citation(inv Execution) (source, error) {
	line, err := argInt(inv.Args, "said_in")
	if err != nil {
		return source{}, err
	}
	speaker, err := argString(inv.Args, "said_by")
	if err != nil {
		return source{}, err
	}

	if line < 1 || line > len(inv.Sources) {
		return source{}, fmt.Errorf("there is no line %d in the conversation in front of you — "+
			"give the number printed at the start of the line this was said on", line)
	}
	src := inv.Sources[line-1]

	if src.mine {
		return source{}, fmt.Errorf("line %d is something you said yourself, and you are not a source "+
			"for what you already know — point at the line where somebody told you this", line)
	}
	if !strings.EqualFold(strings.TrimSpace(speaker), strings.TrimSpace(src.speaker)) {
		return source{}, fmt.Errorf("line %d is %s, not %s — check the number against the name "+
			"and give me the line this was really said on", line, src.speaker, speaker)
	}
	return src, nil
}

// attribute stamps a Claim with who said it and the day they said it. Writes are
// ungated, so this is what lets someone reading the file later tell what the
// server agreed on from what one member asserted once — and, since the name is
// now the speaker's rather than the Trigger's, tell a Person's own account of
// themselves from somebody else's claim about them.
func attribute(claim string, src source) string {
	who := strings.TrimSpace(src.speaker)
	if who == "" {
		who = "someone"
	}
	return fmt.Sprintf("%s _(%s, @%s)_", strings.TrimSpace(claim), src.at.Format("2006-01-02"), who)
}
