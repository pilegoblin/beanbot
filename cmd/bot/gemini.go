package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"google.golang.org/genai"
)

type GeminiPrompter struct {
	secretKey      string
	backstory      string
	chatSession    *genai.Chat
	sessMutex      sync.Mutex
	thinkingBudget int32
}

func NewGeminiPrompter(backstory string) (*GeminiPrompter, error) {
	key, ok := os.LookupEnv("GOOGLE_API_KEY")
	if !ok {
		return nil, errors.New("token for Google API not found")
	}

	return &GeminiPrompter{secretKey: key, backstory: backstory}, nil

}

func (gp *GeminiPrompter) NewPrompt(ctx context.Context, prompt string, imageBytes ...[]byte) ([]string, error) {
	gp.sessMutex.Lock()
	defer gp.sessMutex.Unlock()
	if gp.backstory == "" {
		return nil, errors.New("backstory is empty")
	}

	if prompt == "" {
		return nil, errors.New("prompt is empty")
	}

	// Initialize chat session if not already created
	if gp.chatSession == nil {
		s, err := gp.CreateChatSession(ctx)
		if err != nil {
			return nil, err
		}
		gp.chatSession = s
	}

	parts := []genai.Part{
		{Text: prompt},
	}

	for _, imageByte := range imageBytes {
		parts = append(parts, *genai.NewPartFromBytes(imageByte, "image/jpeg"))
	}

	resp, err := gp.chatSession.SendMessage(ctx, parts...)
	if err != nil {
		return nil, err
	}

	fullResponse := resp.Text()
	chunks := strings.Split(fullResponse, "\n")

	return chunks, nil
}

func (gp *GeminiPrompter) ResetSession(ctx context.Context) error {
	gp.sessMutex.Lock()
	defer gp.sessMutex.Unlock()
	s, err := gp.CreateChatSession(ctx)
	if err != nil {
		return err
	}
	gp.chatSession = s
	return nil
}

func (gp *GeminiPrompter) CreateChatSession(ctx context.Context) (*genai.Chat, error) {

	cc := &genai.ClientConfig{
		APIKey: gp.secretKey,
	}
	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, err
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(gp.backstory, genai.RoleUser),
		ThinkingConfig: &genai.ThinkingConfig{
			ThinkingBudget: &gp.thinkingBudget,
		},
		SafetySettings: []*genai.SafetySetting{{
			Category:  genai.HarmCategoryHarassment,
			Threshold: genai.HarmBlockThresholdBlockNone,
		},
			{
				Category:  genai.HarmCategoryHateSpeech,
				Threshold: genai.HarmBlockThresholdBlockNone,
			},
			{
				Category:  genai.HarmCategorySexuallyExplicit,
				Threshold: genai.HarmBlockThresholdBlockNone,
			},
			{
				Category:  genai.HarmCategoryDangerousContent,
				Threshold: genai.HarmBlockThresholdBlockNone,
			},
		},
	}

	chat, err := client.Chats.Create(ctx, "gemini-2.5-flash", config, nil)
	if err != nil {
		return nil, err
	}

	return chat, nil
}
