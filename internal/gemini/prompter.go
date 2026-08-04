package gemini

import (
	"context"
	"errors"
	"fmt"
	"os"

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
	if !ok {
		return nil, errors.New("token for Google API not found")
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

	out := Response{Text: resp.Text()}
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.FunctionCall != nil {
			out.FunctionCalls = append(out.FunctionCalls, FunctionCall{
				Name: part.FunctionCall.Name,
				Args: part.FunctionCall.Args,
			})
		}
	}
	return out, nil
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
	if text := resp.Text(); text != "" {
		return Image{}, fmt.Errorf("no image was produced: %s", text)
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
