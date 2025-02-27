package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
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

func (gp GeminiPrompter) NewPrompt(prompt string) (string, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(gp.secretKey))
	if err != nil {
		return "", err
	}
	defer client.Close()
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
	prompt = prompt + "\n\n" + gp.backstory

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}

	fullResponse := ""
	parts := resp.Candidates[0].Content.Parts
	for _, part := range parts {
		fullResponse += fmt.Sprint(part)
	}

	return fullResponse, nil
}
