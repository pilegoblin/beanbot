package gemini

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

const (
	// ChatModel answers Triggers and decides which Capabilities to call.
	ChatModel = "gemini-3-flash-preview"
	// ImageModel is Nano Banana — Gemini 2.5 Flash Image. Deliberately not
	// Nano Banana Pro (gemini-3-pro-image-preview), which costs several times
	// more per image and is slower.
	ImageModel = "gemini-2.5-flash-image"
)

// Image is a picture moving in or out of the model, carrying the MIME type
// Discord reported rather than assuming everything is a JPEG.
type Image struct {
	Data []byte
	MIME string
}

// FunctionCall is the model asking for a Capability to be run.
type FunctionCall struct {
	Name string
	Args map[string]any
}

// FunctionResult is what running it produced. Only one of Output or Err is set.
type FunctionResult struct {
	Name   string
	Output string
	Err    string
}

// Response is one reply from the model: prose, tool calls, or both.
type Response struct {
	Text          string
	FunctionCalls []FunctionCall
}

// Prompter is BeanBot's connection to Gemini. It holds no conversation state:
// memory is the Backlog, fetched from Discord per request.
type Prompter struct {
	client    *genai.Client
	backstory string
}

func NewPrompter(ctx context.Context, backstory string) (*Prompter, error) {
	key, ok := os.LookupEnv("GOOGLE_API_KEY")
	if !ok || key == "" {
		return nil, errors.New("GOOGLE_API_KEY is not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: key})
	if err != nil {
		return nil, err
	}

	return &Prompter{client: client, backstory: backstory}, nil
}

// Turn is a single exchange with the model, which may take several round-trips
// as Capabilities are called and their results reported back. It exists only
// for the life of one Trigger.
type Turn struct {
	prompter *Prompter
	tools    []*genai.Tool
	contents []*genai.Content
}

// NewTurn starts an exchange in which the model may call the given Capabilities.
func (p *Prompter) NewTurn(declarations []*genai.FunctionDeclaration) *Turn {
	t := &Turn{prompter: p}
	if len(declarations) > 0 {
		t.tools = []*genai.Tool{{FunctionDeclarations: declarations}}
	}
	return t
}

// Ask puts the rendered backlog and any attached images to the model.
func (t *Turn) Ask(ctx context.Context, text string, images []Image) (Response, error) {
	parts := []*genai.Part{{Text: text}}
	for _, img := range images {
		parts = append(parts, genai.NewPartFromBytes(img.Data, img.MIME))
	}

	t.contents = append(t.contents, genai.NewContentFromParts(parts, genai.RoleUser))
	return t.generate(ctx)
}

// Report feeds Capability results back so the model can narrate what happened
// or decide what to do next.
func (t *Turn) Report(ctx context.Context, results []FunctionResult) (Response, error) {
	parts := make([]*genai.Part, 0, len(results))
	for _, r := range results {
		response := map[string]any{"output": r.Output}
		if r.Err != "" {
			response = map[string]any{"error": r.Err}
		}
		parts = append(parts, &genai.Part{
			FunctionResponse: &genai.FunctionResponse{Name: r.Name, Response: response},
		})
	}

	t.contents = append(t.contents, genai.NewContentFromParts(parts, genai.RoleUser))
	return t.generate(ctx)
}

func (t *Turn) generate(ctx context.Context) (Response, error) {
	resp, err := t.prompter.client.Models.GenerateContent(ctx, ChatModel, t.contents, &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(t.prompter.backstory, genai.RoleUser),
		Tools:             t.tools,
		SafetySettings:    permissiveSafety(),
	})
	if err != nil {
		return Response{}, err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return Response{}, errors.New("the model returned nothing")
	}

	// Keep the model's own turn in the history so it can see the calls it
	// already made when results come back.
	t.contents = append(t.contents, resp.Candidates[0].Content)

	// Assembled by hand rather than via resp.Text(), which logs a warning on
	// every response that also carries a function call — i.e. every response
	// where BeanBot actually does something.
	var out Response
	var text strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		switch {
		case part.FunctionCall != nil:
			out.FunctionCalls = append(out.FunctionCalls, FunctionCall{
				Name: part.FunctionCall.Name,
				Args: part.FunctionCall.Args,
			})
		case part.Text != "" && !part.Thought:
			text.WriteString(part.Text)
		}
	}

	out.Text = text.String()
	return out, nil
}

// compactionInstruction replaces the Backstory for Compaction, deliberately.
// The Backstory's whole job is to make BeanBot say things in a distinctive
// voice; aimed at the one operation whose success criterion is fidelity, it
// produces a Memory rewritten to be funnier. Compaction overwrites the only
// copy, so there is nothing left to compare against afterwards.
const compactionInstruction = `You are compacting a markdown file of notes that a Discord bot keeps about a server, because it has grown too large. Rewrite it at roughly half the length.

Rules:
- Preserve every distinct fact. Merge duplicate or near-duplicate entries about the same subject into a single entry.
- Never invent, embellish, reinterpret or joke. Keep the original wording of each fact wherever you can.
- Drop entries that record passing chatter rather than something lasting about the server or its members.
- Keep the trailing _(date, @author)_ attribution on every entry. When merging, keep the most recent date.
- Keep the existing structure: "## Section" headings with "- entry" bullets beneath them.
- Never add a "## People" heading. Notes about individual people are kept separately, are not part of what you are being given, and must not be invented here.
- Output the markdown document and nothing else — no preamble, no commentary, no code fences.`

// Compact rewrites an oversized Memory smaller. It bypasses Turn: there is no
// conversation, no Backstory and no tools, only the document.
func (p *Prompter) Compact(ctx context.Context, document string) (string, error) {
	resp, err := p.client.Models.GenerateContent(ctx, ChatModel,
		[]*genai.Content{genai.NewContentFromText(document, genai.RoleUser)},
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(compactionInstruction, genai.RoleUser),
			SafetySettings:    permissiveSafety(),
		})
	if err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", errors.New("the model returned nothing")
	}

	var out strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" && !part.Thought {
			out.WriteString(part.Text)
		}
	}

	return stripCodeFence(out.String()), nil
}

// stripCodeFence unwraps a ```markdown block. The instruction forbids fences
// and the model supplies them anyway often enough that the fence would
// otherwise be stored as the first line of the Memory.
func stripCodeFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "```") {
		return s
	}

	_, rest, found := strings.Cut(trimmed, "\n")
	if !found {
		return s
	}
	if end := strings.LastIndex(rest, "```"); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest)
}

// GenerateImage draws with Nano Banana. Supplying inputs turns generation into
// editing — the same call, with the source pictures attached.
func (p *Prompter) GenerateImage(ctx context.Context, prompt string, inputs []Image) (Image, error) {
	parts := []*genai.Part{{Text: prompt}}
	for _, in := range inputs {
		parts = append(parts, genai.NewPartFromBytes(in.Data, in.MIME))
	}

	resp, err := p.client.Models.GenerateContent(ctx, ImageModel,
		[]*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)},
		&genai.GenerateContentConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			SafetySettings:     permissiveSafety(),
		})
	if err != nil {
		return Image{}, err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return Image{}, errors.New("the image model returned nothing")
	}

	for _, part := range resp.Candidates[0].Content.Parts {
		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			return Image{Data: part.InlineData.Data, MIME: part.InlineData.MIMEType}, nil
		}
	}

	// The model answered in words instead of pictures, usually to refuse.
	var refusal strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" && !part.Thought {
			refusal.WriteString(part.Text)
		}
	}
	if refusal.Len() > 0 {
		return Image{}, fmt.Errorf("no image was produced: %s", refusal.String())
	}
	return Image{}, errors.New("no image was produced")
}

// permissiveSafety disables Gemini's content filters. BeanBot has a persona
// that the default thresholds interfere with.
func permissiveSafety() []*genai.SafetySetting {
	categories := []genai.HarmCategory{
		genai.HarmCategoryHarassment,
		genai.HarmCategoryHateSpeech,
		genai.HarmCategorySexuallyExplicit,
		genai.HarmCategoryDangerousContent,
	}

	settings := make([]*genai.SafetySetting, 0, len(categories))
	for _, c := range categories {
		settings = append(settings, &genai.SafetySetting{
			Category:  c,
			Threshold: genai.HarmBlockThresholdOff,
		})
	}
	return settings
}
