package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

var (
	Token string
)

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()
}

func main() {
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	dg.AddHandler(messageCreate)

	dg.Identify.Intents = discordgo.IntentsGuildMessages

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	fmt.Println("Beanbot Active")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	isTikTok, URL := containsURL(m.Content)
	if isTikTok {
		err := downloadTikTok(URL, s, m)
		if err != nil {
			fmt.Println(err)
			// just returning to not crash the bot or anything
			// leaves the message in-tact and doesn't remove the embed
			return
		}
		// remove embed
		_, err = s.Request("PATCH", discordgo.EndpointChannelMessage(m.ChannelID, m.ID), map[string]any{"flags": 4})

		if err != nil {
			fmt.Println("unable to remove embed:", err)
			return
		}

	}
}
