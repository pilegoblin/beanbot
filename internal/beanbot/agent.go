package beanbot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
	"google.golang.org/genai"
)

// maxIterations bounds how many times the model may call Capabilities within
// one Trigger. Chaining is what makes "draw a poster and make the event" work;
// without a ceiling, a looping model spends without limit.
const maxIterations = 5

// conversation is the model side of a turn. *gemini.Turn satisfies it; tests
// script it directly.
type conversation interface {
	Ask(ctx context.Context, text string, images []gemini.Image) (gemini.Response, error)
	Report(ctx context.Context, results []gemini.FunctionResult) (gemini.Response, error)
}

// agent runs one Trigger to completion: ask the model, execute whatever
// Capabilities it calls, feed the results back, repeat until it has prose.
type agent struct {
	capabilities map[string]Capability
	// offerings keeps the Capabilities in declaration order with their schemas,
	// which are built once: which Capabilities are On Offer varies per Trigger,
	// but what each one looks like does not.
	offerings []offering
}

type offering struct {
	capability  Capability
	declaration *genai.FunctionDeclaration
}

func newAgent(caps []Capability) *agent {
	a := &agent{capabilities: make(map[string]Capability, len(caps))}
	for _, c := range caps {
		d := c.Declaration()
		a.capabilities[d.Name] = c
		a.offerings = append(a.offerings, offering{capability: c, declaration: d})
	}
	return a
}

// declare is the set of tools the model is shown for one Trigger. Anything the
// Trigger did not Cue is left out entirely rather than declared and refused.
func (a *agent) declare(cue Cueing) []*genai.FunctionDeclaration {
	declarations := make([]*genai.FunctionDeclaration, 0, len(a.offerings))
	for _, o := range a.offerings {
		if onOffer(o.capability, cue) {
			declarations = append(declarations, o.declaration)
		}
	}
	return declarations
}

// run returns BeanBot's reply and any files its Capabilities produced.
//
// Capability failures are reported back to the model rather than returned as
// errors: BeanBot should explain a refusal in character, not fall silent.
func (a *agent) run(ctx context.Context, conv conversation, backlog string, images []gemini.Image, inv Execution, cue Cueing) (string, []*discordgo.File, error) {
	resp, err := conv.Ask(ctx, backlog, images)
	if err != nil {
		return "", nil, err
	}

	var files []*discordgo.File
	remaining := &budget{spent: make(map[Medium]bool)}

	// The opening Ask is the first round-trip, so the loop may spend only the
	// remaining maxIterations-1.
	for i := 1; i < maxIterations && len(resp.FunctionCalls) > 0; i++ {
		results := make([]gemini.FunctionResult, 0, len(resp.FunctionCalls))

		for _, call := range resp.FunctionCalls {
			result, produced, err := a.invoke(ctx, call, inv, remaining, cue)
			if err != nil {
				log.Printf("capability %s: %v", call.Name, err)
				results = append(results, gemini.FunctionResult{Name: call.Name, Err: err.Error()})
				continue
			}
			files = append(files, produced...)
			results = append(results, gemini.FunctionResult{Name: call.Name, Output: result})
		}

		if resp, err = conv.Report(ctx, results); err != nil {
			return "", files, err
		}
	}

	// The model may run out of round-trips mid-tool-call, or return nothing at
	// all. Either way BeanBot must not fall silent.
	if strings.TrimSpace(resp.Text) == "" {
		return "i got tangled up doing that one — try asking again?", files, nil
	}
	return resp.Text, files, nil
}

// budget is what one Trigger is allowed to spend: one change to the Guild, and
// one file of each Medium. The two are counted separately because they limit
// different things — the first bounds what BeanBot does to the server, the
// second bounds what it costs.
type budget struct {
	mutated bool
	spent   map[Medium]bool
}

// check reports whether this Capability has already had its turn, saying why
// in terms the model can relay to the channel. The Medium refusal tells the
// model not to retry rather than to "ask again": it is being read inside a
// turn that has round-trips left, and an invitation spends them all on calls
// that will be refused identically.
func (b *budget) check(c Capability) error {
	if c.Mutating() && b.mutated {
		return errors.New("only one change to the server is allowed per message — ask again for the next one")
	}
	if m := c.Medium(); m != NoMedium && b.spent[m] {
		return fmt.Errorf("you have already made one %s and only one is allowed per message — "+
			"do not call this again in this turn, just say what you were going to say", m)
	}
	return nil
}

// spend records a successful call. It runs only after the Capability returns,
// so a refused or malformed attempt leaves room for a corrected retry in the
// same turn.
func (b *budget) spend(c Capability) {
	if c.Mutating() {
		b.mutated = true
	}
	if m := c.Medium(); m != NoMedium {
		b.spent[m] = true
	}
}

// invoke resolves, gates and runs a single Capability. The budget is threaded
// through so that one Trigger cannot spend without limit.
func (a *agent) invoke(ctx context.Context, call gemini.FunctionCall, inv Execution, remaining *budget, cue Cueing) (string, []*discordgo.File, error) {
	// A Capability the Trigger did not Cue was never declared, and answers here
	// as though it does not exist — which for this Trigger it does not. The
	// check is repeated rather than assumed because a model that has been told
	// to call something can emit the call whether or not it was offered one.
	capability, ok := a.capabilities[call.Name]
	if !ok || !onOffer(capability, cue) {
		return "", nil, fmt.Errorf("there is no capability called %q", call.Name)
	}

	if err := remaining.check(capability); err != nil {
		return "", nil, err
	}

	if err := checkGate(capability, inv); err != nil {
		return "", nil, err
	}

	inv.Args = call.Args
	result, err := capability.Execute(ctx, inv)
	if err != nil {
		return "", nil, err
	}

	remaining.spend(capability)
	return result.Summary, result.Files, nil
}
