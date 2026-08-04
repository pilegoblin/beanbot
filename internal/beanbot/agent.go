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
	declarations []*genai.FunctionDeclaration
}

func newAgent(caps []Capability) *agent {
	a := &agent{capabilities: make(map[string]Capability, len(caps))}
	for _, c := range caps {
		d := c.Declaration()
		a.capabilities[d.Name] = c
		a.declarations = append(a.declarations, d)
	}
	return a
}

// run returns BeanBot's reply and any files its Capabilities produced.
//
// Capability failures are reported back to the model rather than returned as
// errors: BeanBot should explain a refusal in character, not fall silent.
func (a *agent) run(ctx context.Context, conv conversation, backlog string, images []gemini.Image, inv Execution) (string, []*discordgo.File, error) {
	resp, err := conv.Ask(ctx, backlog, images)
	if err != nil {
		return "", nil, err
	}

	var files []*discordgo.File
	mutated := false

	// The opening Ask is the first round-trip, so the loop may spend only the
	// remaining maxIterations-1.
	for i := 1; i < maxIterations && len(resp.FunctionCalls) > 0; i++ {
		results := make([]gemini.FunctionResult, 0, len(resp.FunctionCalls))

		for _, call := range resp.FunctionCalls {
			result, produced, err := a.invoke(ctx, call, inv, &mutated)
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

// invoke resolves, gates and runs a single Capability. mutated is threaded
// through so at most one guild-mutating Capability runs per Trigger.
func (a *agent) invoke(ctx context.Context, call gemini.FunctionCall, inv Execution, mutated *bool) (string, []*discordgo.File, error) {
	capability, ok := a.capabilities[call.Name]
	if !ok {
		return "", nil, fmt.Errorf("there is no capability called %q", call.Name)
	}

	if capability.Mutating() && *mutated {
		return "", nil, errors.New("only one change to the server is allowed per message — ask again for the next one")
	}

	if err := checkGate(capability, inv); err != nil {
		return "", nil, err
	}

	inv.Args = call.Args
	result, err := capability.Execute(ctx, inv)
	if err != nil {
		return "", nil, err
	}

	// The budget is spent only once the guild has actually changed. A refused
	// or malformed attempt must leave room for a corrected retry in this turn.
	if capability.Mutating() {
		*mutated = true
	}
	return result.Summary, result.Files, nil
}
