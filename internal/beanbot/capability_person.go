package beanbot

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// rememberPerson records something about a human in the Guild's Roster, whether
// or not that human is in the Guild.
//
// Ungated, for the same reason remember is: a Roster only moderators may write
// to is a mod-maintained wiki rather than BeanBot learning who the server talks
// about. What that costs is the same too — the notes are member-authored text
// injected into every later prompt, so they are fenced as fallible observations
// and can never reach a Gate.
type rememberPerson struct{ memory *Memory }

func (rememberPerson) RequiredPermission() int64 { return 0 }

// Mutating reports Guild-mutating, and a note about somebody is not: it costs
// no API call and changes nothing in Discord.
func (rememberPerson) Mutating() bool { return false }

func (rememberPerson) Medium() Medium { return NoMedium }

func (rememberPerson) Cues() []string { return nil }

func (rememberPerson) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: "remember_person",
		Description: "Keep a note about a person in your long-term notes. Use it for anyone " +
			"you learn something about — members of this server, and people who are only " +
			"talked about: someone's partner, a streamer the server follows, a person in a " +
			"screenshot or a link somebody shows you. Record them the first time you learn " +
			"anything about them, however small, so you still know who they are months from " +
			"now. Do not record a bare mention that says nothing about the person. Also use " +
			"this to correct a note about them that is now wrong.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"name": {
					Type: genai.TypeString,
					Description: "Who this is about, by the fullest name you know for them. If " +
						"you already have notes about this person, reuse exactly the name they " +
						"are filed under rather than a new spelling of it.",
				},
				"claim": {
					Type: genai.TypeString,
					Description: "The thing to record about them, as one self-contained " +
						"sentence. Write it so it still makes sense months from now to someone " +
						"who cannot see this conversation — name them rather than saying " +
						"\"he\" or \"they\".",
				},
				"said_in": {
					Type: genai.TypeInteger,
					Description: "The number printed at the start of the line where this was said. " +
						"Your notes record who told you a thing and the day they told you, so this " +
						"must be the line it was actually said on, not the line you are replying " +
						"to. Where more than one line would do, prefer the one where the person " +
						"this is about told you themselves — their own account of themselves is " +
						"worth more than somebody else's claim about them. You cannot cite one of " +
						"your own lines.",
				},
				"said_by": {
					Type: genai.TypeString,
					Description: "The name shown on that line, copied exactly. It is checked against " +
						"the line number, so a miscount is caught rather than filed under whoever " +
						"happened to speak next.",
				},
				"aliases": {
					Type:  genai.TypeArray,
					Items: &genai.Schema{Type: genai.TypeString},
					Description: "Optional. Other names this person is called — a nickname, a " +
						"shortening, a different spelling — so you find them again when the " +
						"channel uses one of those instead.",
				},
				"replaces": {
					Type: genai.TypeString,
					Description: "Optional. To correct something already recorded about this " +
						"person, quote enough of that existing note to identify it and it will " +
						"be rewritten in place. Leave this out to record something new.",
				},
			},
			Required: []string{"name", "claim", "said_in", "said_by"},
		},
	}
}

func (r rememberPerson) Execute(_ context.Context, inv Execution) (Result, error) {
	name, err := argString(inv.Args, "name")
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

	ch := personChange{
		Name:     name,
		Claim:    attribute(claim, src),
		Aliases:  optionalStrings(inv.Args, "aliases"),
		ID:       inv.identify(name),
		Replaces: optionalString(inv.Args, "replaces"),
	}
	if err := r.memory.RecordPerson(inv.GuildID, ch); err != nil {
		return Result{}, err
	}

	return Result{Summary: fmt.Sprintf("Noted about %s: %s", name, claim)}, nil
}

// identify resolves a name to a Discord snowflake, and only ever from something
// Discord itself said: the Trigger came from them, or the message @-mentioned
// them and Discord resolved the id.
//
// Matching the name against everyone who spoke in the Backlog would catch more
// cases and misfire exactly where it hurts — a Steve in the channel and a Steve
// on Facebook, quietly declared one human. No id is recoverable, because the
// Person still matches by name and the id can arrive later. A wrong one is not.
func (inv Execution) identify(name string) string {
	if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(inv.Author)) {
		return inv.UserID
	}
	for _, m := range inv.Mentions {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(m.Name)) {
			return m.ID
		}
	}
	return ""
}

// mergePeople folds two entries together once they turn out to be the same
// person. Duplicates are certain rather than possible: BeanBot decides for
// itself whether a name is somebody it already knows, so sooner or later it
// files a second Steve.
//
// Ungated, unlike a deletion would be, because a merge destroys nothing — every
// Claim moves across with its Attribution, and the absorbed name lives
// on as an alias. The worst it can do is weld two unrelated people together,
// which is visible in the file and repairable.
type mergePeople struct{ memory *Memory }

func (mergePeople) RequiredPermission() int64 { return 0 }
func (mergePeople) Mutating() bool            { return false }
func (mergePeople) Medium() Medium            { return NoMedium }
func (mergePeople) Cues() []string            { return nil }

func (mergePeople) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: "merge_people",
		Description: "Fold two entries in your notes together when you realise they are the " +
			"same person under different names. Everything you know about them ends up in one " +
			"place and nothing is lost. Only use this when you are confident they really are " +
			"the same person.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"from": {
					Type: genai.TypeString,
					Description: "The name of the entry to fold in — usually the sparser or " +
						"less complete of the two. It survives as another name for the person.",
				},
				"into": {
					Type:        genai.TypeString,
					Description: "The name of the entry to keep, which everything is moved into.",
				},
			},
			Required: []string{"from", "into"},
		},
	}
}

func (m mergePeople) Execute(_ context.Context, inv Execution) (Result, error) {
	from, err := argString(inv.Args, "from")
	if err != nil {
		return Result{}, err
	}
	into, err := argString(inv.Args, "into")
	if err != nil {
		return Result{}, err
	}

	if err := m.memory.Merge(inv.GuildID, from, into); err != nil {
		return Result{}, err
	}
	return Result{Summary: fmt.Sprintf("%s and %s are the same person; the notes are now one entry.", from, into)}, nil
}
