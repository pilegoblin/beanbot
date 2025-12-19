package beanbot

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pilegoblin/beanbot/internal/gemini"
)

type BeanBot struct {
	session  *discordgo.Session
	prompter *gemini.Prompter
}

func NewBot(ctx context.Context, prompter *gemini.Prompter) (*BeanBot, error) {
	key, ok := os.LookupEnv("DISCORD_API_KEY")
	if !ok {
		return nil, errors.New("token for Discord API not found")
	}

	dg, err := discordgo.New("Bot " + key)
	if err != nil {
		return nil, err
	}

	bb := &BeanBot{
		session:  dg,
		prompter: prompter,
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages
	dg.AddHandler(bb.chatWithBot(ctx))

	return bb, nil
}

func (bb *BeanBot) Start() error {
	err := bb.session.Open()
	if err != nil {
		return err
	}
	defer func() {
		err := bb.session.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	wait := make(chan os.Signal, 1)
	signal.Notify(wait, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	log.Println("Beanbot Active")

	//blocks indefinitely letting the bot run
	<-wait

	return nil
}

// Sets the bot's status to 'Playing <status>'
func (bb *BeanBot) SetStatus(status string) {
	bb.session.AddHandler(func(s *discordgo.Session, event *discordgo.Ready) {
		err := s.UpdateGameStatus(0, status)
		if err != nil {
			log.Println(err)
		}
	})
}

func (bb *BeanBot) chatWithBot(ctx context.Context) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}
		if strings.Contains(strings.ToLower(m.Content), "!bbreset") {
			err := bb.prompter.ResetSession(ctx)
			if err != nil {
				log.Println(err)
			}
		}
		if !strings.Contains(strings.ToLower(m.Content), "beanbot") {
			return
		}

		if hasAllImageAttachments(m.Attachments) {
			bb.handleImage(ctx, s, m)
		} else {
			bb.handleText(ctx, s, m)
		}
	}
}

func hasAllImageAttachments(attachments []*discordgo.MessageAttachment) bool {
	if len(attachments) == 0 {
		return false
	}
	for _, attachment := range attachments {
		if !strings.Contains(strings.ToLower(attachment.ContentType), "image") {
			return false
		}
	}
	return true
}

func (bb *BeanBot) handleImage(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	c, err := AsyncType(s, m.ChannelID)
	if err != nil {
		log.Println(err)
		return
	}
	defer c.Stop()

	urls := make([]string, len(m.Attachments))
	for i, attachment := range m.Attachments {
		urls[i] = attachment.URL
	}

	imageBytes := make([][]byte, len(urls))
	for i, url := range urls {
		imageResp, err := http.Get(url)
		if err != nil {
			log.Println(err)
			return
		}
		imageBytes[i], err = io.ReadAll(imageResp.Body)
		if err != nil {
			log.Println(err)
			return
		}

		err = imageResp.Body.Close()
		if err != nil {
			log.Println(err)
		}
	}

	resp, err := bb.prompter.NewPrompt(ctx, m.Content, imageBytes...)
	if err != nil {
		log.Println(err)
		return
	}

	err = SendChunks(s, m.ChannelID, resp)
	if err == nil {
		return
	}
	log.Println(err)
}

func (bb *BeanBot) handleText(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	c, err := AsyncType(s, m.ChannelID)
	if err != nil {
		log.Println(err)
		return
	}
	defer c.Stop()

	// generate the prompt
	resp, err := bb.prompter.NewPrompt(ctx, m.Content)
	if err != nil {
		log.Println(err)
		return
	}

	err = SendChunks(s, m.ChannelID, resp)
	if err == nil {
		return
	}
	log.Println(err)

	// as a final failsafe, send an "error message"
	if sentMessage, err := s.ChannelMessageSend(m.ChannelID, "ERROR! ERROR!"); err != nil {
		log.Println(err)
	} else {
		log.Println(sentMessage)
		return
	}
}

func AsyncType(s *discordgo.Session, channelID string) (*time.Ticker, error) {
	// send a typing status once at the start
	err := s.ChannelTyping(channelID)
	if err != nil {
		log.Println(err)
	}
	// then send a typing status every 5 seconds if the channel is still active
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			err := s.ChannelTyping(channelID)
			if err != nil {
				log.Println(err)
			}
		}
	}()

	return ticker, nil
}

func SendChunks(s *discordgo.Session, channelID string, chunks []string) error {
	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		if _, err := s.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
	}
	return nil
}
