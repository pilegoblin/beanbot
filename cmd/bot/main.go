package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/pilegoblin/beanbot/internal/beanbot"
	"github.com/pilegoblin/beanbot/internal/gemini"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	backstory := os.Getenv("BEANBOT_BACKSTORY")
	if backstory == "" {
		log.Fatal("BACKSTORY environment variable is not set")
	}

	// Initialize the Gemini prompter
	prompter, err := gemini.NewPrompter(backstory)
	if err != nil {
		log.Fatal(err)
	}

	bb, err := beanbot.NewBot(context.Background(), prompter)
	if err != nil {
		log.Fatal(err)
	}

	bb.SetStatus("BeanBot")

	err = bb.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("BeanBot has exited.")
}
