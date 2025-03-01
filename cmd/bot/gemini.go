package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

var (
	gemOnce   sync.Once
	sessMutex sync.Mutex
	session   *genai.ChatSession
)

type GeminiPrompter struct {
	secretKey string // GOOGLE_API_KEY
	backstory string // context given to each prompt when making an answer
}

func NewGeminiPrompter(backstory string) (*GeminiPrompter, error) {
	key, ok := os.LookupEnv("GOOGLE_API_KEY")
	if !ok {
		return nil, errors.New("token for Google API not found")
	}

	return &GeminiPrompter{secretKey: key, backstory: backstory}, nil

}

func (gp GeminiPrompter) NewPrompt(ctx context.Context, prompt string) (*string, error) {
	sessMutex.Lock()
	defer sessMutex.Unlock()
	if gp.backstory == "" {
		return nil, errors.New("backstory is empty")
	}

	if prompt == "" {
		return nil, errors.New("prompt is empty")
	}

	gemOnce.Do(func() {
		s, err := gp.CreateSession(ctx)
		if err != nil {
			panic(err)
		}
		session = s
	})

	resp, err := session.SendMessage(ctx, genai.Text(prompt))
	if err != nil {
		return nil, err
	}

	fullResponse := ""
	parts := resp.Candidates[0].Content.Parts
	for _, part := range parts {
		fullResponse += fmt.Sprint(part.(genai.Text))
	}

	return &fullResponse, nil
}

func (gp *GeminiPrompter) ResetSession(ctx context.Context) error {
	sessMutex.Lock()
	defer sessMutex.Unlock()
	s, err := gp.CreateSession(ctx)
	if err != nil {
		return err
	}
	session = s
	return nil
}

func (gp *GeminiPrompter) CreateSession(ctx context.Context) (*genai.ChatSession, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(gp.secretKey))
	if err != nil {
		return nil, err
	}
	model := client.GenerativeModel("gemini-2.0-flash")
	model.SafetySettings = []*genai.SafetySetting{
		{
			Category:  genai.HarmCategoryHarassment,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryHateSpeech,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategorySexuallyExplicit,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockNone,
		},
	}

	s := model.StartChat()

	// send the backstory once
	_, err = s.SendMessage(ctx, genai.Text(gp.backstory))
	if err != nil {
		return nil, err
	}
	return s, nil
}
