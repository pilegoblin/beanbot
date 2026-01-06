package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"google.golang.org/genai"
)

const (
	GeminiModel = "gemini-3-flash-preview"
)

type Prompter struct {
	secretKey      string
	backstory      string
	chatSession    *genai.Chat
	sessMutex      sync.Mutex
	thinkingBudget int32
}

func NewPrompter(backstory string) (*Prompter, error) {
	key, ok := os.LookupEnv("GOOGLE_API_KEY")
	if !ok {
		return nil, errors.New("token for Google API not found")
	}

	return &Prompter{secretKey: key, backstory: backstory}, nil
}

func (gp *Prompter) NewPromptFromDiscordMessage(ctx context.Context, m *discordgo.MessageCreate, imageBytes ...[]byte) ([]string, error) {
	gp.sessMutex.Lock()
	defer gp.sessMutex.Unlock()

	// Initialize chat session if not already created
	if gp.chatSession == nil {
		s, err := gp.CreateChatSession(ctx)
		if err != nil {
			return nil, err
		}
		gp.chatSession = s
	}

	jsonBytes, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	jsonString := string(jsonBytes)

	parts := []genai.Part{
		{Text: jsonString},
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

func (gp *Prompter) ResetSession(ctx context.Context) error {
	gp.sessMutex.Lock()
	defer gp.sessMutex.Unlock()
	s, err := gp.CreateChatSession(ctx)
	if err != nil {
		return err
	}
	gp.chatSession = s
	return nil
}

func (gp *Prompter) CreateChatSession(ctx context.Context) (*genai.Chat, error) {

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
		SafetySettings: []*genai.SafetySetting{
			{
				Category:  genai.HarmCategoryHarassment,
				Threshold: genai.HarmBlockThresholdOff,
			},
			{
				Category:  genai.HarmCategoryHateSpeech,
				Threshold: genai.HarmBlockThresholdOff,
			},
			{
				Category:  genai.HarmCategorySexuallyExplicit,
				Threshold: genai.HarmBlockThresholdOff,
			},
			{
				Category:  genai.HarmCategoryDangerousContent,
				Threshold: genai.HarmBlockThresholdOff,
			},
		},
	}

	chat, err := client.Chats.Create(ctx, GeminiModel, config, nil)
	if err != nil {
		return nil, err
	}

	return chat, nil
}
