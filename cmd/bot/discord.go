package main

import (
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

func NewBot() (*BeanBot, error) {
	key, ok := os.LookupEnv("DISCORD_API_KEY")
	if !ok {
		return nil, errors.New("token for Discord API not found")
	}

	dg, err := discordgo.New("Bot " + key)
	if err != nil {
		return nil, err
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages
	dg.AddHandler(chatWithBot)

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

func chatWithBot(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	if strings.Contains(strings.ToLower(m.Content), "beanbot") {
		// build bot for prompting
		gp, err := NewGeminiPrompter("You are a hyper-intelligent AI named BeanBot that is made entirely out of beans. " +
			"Respond in 1 sentence. Be silly.")
		if err != nil {
			log.Println(err)
			return
		}

		// attempt to send the prompt
		resp, err := gp.NewPrompt(m.Content)
		if err == nil {
			s.ChannelTyping(m.ChannelID)
			s.ChannelMessageSend(m.ChannelID, resp)
			return
		}
		// if unable to respond to the provided prompt, try to send an apology
		resp, err = gp.NewPrompt("BeanBot, please say you're sorry and sincerely apologize for not being able to speak.")
		if err == nil {
			s.ChannelTyping(m.ChannelID)
			s.ChannelMessageSend(m.ChannelID, resp)
			return
		}
		// as a final failsafe, send a message
		s.ChannelMessageSend(m.ChannelID, "WOW!")
	}
}
