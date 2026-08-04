package gemini

import (
	"context"
	"os"
	"testing"

	"google.golang.org/genai"
)

// These tests call Gemini for real. They are skipped unless GOOGLE_API_KEY is
// set, so the ordinary `go test ./...` stays offline and free. Run them after
// upgrading the genai SDK or changing a model constant — those are the changes
// no amount of compiling will validate.
//
//	set -a && . ./.env && set +a && go test ./internal/gemini/ -run Live -v
func liveOrSkip(t *testing.T) *Prompter {
	t.Helper()
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set; skipping live Gemini test")
	}

	p, err := NewPrompter(context.Background(), "You are a terse test fixture. Answer in one word.")
	if err != nil {
		t.Fatalf("creating prompter: %v", err)
	}
	return p
}

func TestLiveChatModelAnswers(t *testing.T) {
	p := liveOrSkip(t)

	resp, err := p.NewTurn(nil).Ask(context.Background(), "Say the word: ready", nil)
	if err != nil {
		t.Fatalf("%s rejected the request: %v", ChatModel, err)
	}
	if resp.Text == "" {
		t.Errorf("%s returned no text", ChatModel)
	}
	t.Logf("%s replied: %q", ChatModel, resp.Text)
}

func TestLiveChatModelCallsAFunction(t *testing.T) {
	// Function calling is the whole action spine; if the SDK's tool wiring
	// broke, beanbot would silently lose every Capability while still chatting
	// happily.
	p := liveOrSkip(t)

	declaration := &genai.FunctionDeclaration{
		Name:        "create_event",
		Description: "Schedule an event in this server.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"name": {Type: genai.TypeString, Description: "Title of the event."},
			},
			Required: []string{"name"},
		},
	}

	turn := p.NewTurn([]*genai.FunctionDeclaration{declaration})
	resp, err := turn.Ask(context.Background(),
		"Please schedule an event called Smash Night. Use your tools.", nil)
	if err != nil {
		t.Fatalf("tool-calling request failed: %v", err)
	}
	if len(resp.FunctionCalls) == 0 {
		t.Fatalf("expected a function call, got text only: %q", resp.Text)
	}
	if got := resp.FunctionCalls[0].Name; got != "create_event" {
		t.Errorf("called %q, want create_event", got)
	}

	// Reporting the result back is the other half of the loop.
	final, err := turn.Report(context.Background(), []FunctionResult{
		{Name: "create_event", Output: "Created event \"Smash Night\"."},
	})
	if err != nil {
		t.Fatalf("reporting the result failed: %v", err)
	}
	t.Logf("narrated: %q", final.Text)
}

func TestLiveImageModelDraws(t *testing.T) {
	// Nano Banana is a different model with a different response shape; a
	// working chat model says nothing about it.
	p := liveOrSkip(t)

	img, err := p.GenerateImage(context.Background(), "A single plain brown bean on a white background.", nil)
	if err != nil {
		t.Fatalf("%s failed to draw: %v", ImageModel, err)
	}
	if len(img.Data) == 0 {
		t.Fatalf("%s returned an empty image", ImageModel)
	}
	t.Logf("%s drew %d bytes of %s", ImageModel, len(img.Data), img.MIME)
}
