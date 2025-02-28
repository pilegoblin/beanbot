package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

type BeanBot struct {
	session *discordgo.Session
}

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
		if m.Author.ID == s.State.User.ID {
			return
		}
		if !strings.Contains(strings.ToLower(m.Content), "beanbot") {
			return
		}

		// create the bot instance
		gp, err := NewGeminiPrompter("You are a genius supercomputer made entirely out of beans. Your name is BeanBot. " +
			"You are a helpful assistant. Talk like a huge nerd. Keep responses short.")
		if err != nil {
			log.Println(err)
			return
		}

		// generate the prompt
		resp, err := gp.NewPrompt(ctx, m.Content)
		if err != nil {
			log.Println(err)
		} else {
			TypeAndSend(s, m.ChannelID, *resp)
			return
		}

		// if unable generate a prompt, generate a fallback
		resp, err = gp.NewPrompt(ctx, "BeanBot, please say you're sorry and sincerely apologize for not being able to speak.")
		if err != nil {
			log.Println(err)
		} else {
			TypeAndSend(s, m.ChannelID, *resp)
			return
		}

		// as a final failsafe, send an "error message"
		TypeAndSend(s, m.ChannelID, "ERROR! ERROR!")
	}

}

func TypeAndSend(s *discordgo.Session, channelID string, message string) {
	if err := s.ChannelTyping(channelID); err != nil {
		log.Println(err)
	}

	if _, err := s.ChannelMessageSend(channelID, message); err != nil {
		log.Println(err)
	}
}
