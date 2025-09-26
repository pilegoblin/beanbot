package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

type BeanBot struct {
	session *discordgo.Session
}

var (
	gemPrompter *GeminiPrompter
	once        sync.Once
)

func NewBot(ctx context.Context) (*BeanBot, error) {
	key, ok := os.LookupEnv("DISCORD_API_KEY")
	if !ok {
		return nil, errors.New("token for Discord API not found")
	}

	dg, err := discordgo.New("Bot " + key)
	if err != nil {
		return nil, err
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages
	dg.AddHandler(chatWithBot(ctx))

	return &BeanBot{session: dg}, nil

}

func (bb *BeanBot) Start() error {

	err := bb.session.Open()
	if err != nil {
		return err
	}
	defer bb.session.Close()

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
		s.UpdateGameStatus(0, status)
	})
}

func chatWithBot(ctx context.Context) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		once.Do(func() {
			// create the bot instance
			g, err := NewGeminiPrompter("You are a genius supercomputer made entirely out of beans. Your name is BeanBot. " +
				"You are a helpful assistant. No symbols other than commas and periods, no markdown, no formatting, no nothing. Just the plain text of the response." +
				"Keep responses short, 1000 words or less, 2 paragraphs.")
			if err != nil {
				log.Println(err)
				return
			}
			gemPrompter = g
		})
		if m.Author.ID == s.State.User.ID {
			return
		}
		if strings.Contains(strings.ToLower(m.Content), "!bbreset") {
			gemPrompter.ResetSession(ctx)
		}
		if !strings.Contains(strings.ToLower(m.Content), "beanbot") {
			return
		}

		done, err := AsyncType(s, m.ChannelID)
		if err != nil {
			log.Println(err)
			return
		}
		defer func() {
			done <- true
		}()

		// generate the prompt
		resp, err := gemPrompter.NewPrompt(ctx, m.Content)
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

}

func AsyncType(s *discordgo.Session, channelID string) (chan bool, error) {
	ticker := time.NewTicker(time.Second)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
				return
			case <-ticker.C:
				err := s.ChannelTyping(channelID)
				if err != nil {
					log.Println(err)
				}
			}
		}
	}()
	return done, nil
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
