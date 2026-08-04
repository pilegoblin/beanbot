package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/pilegoblin/beanbot/internal/beanbot"
	"github.com/pilegoblin/beanbot/internal/gemini"
)

// defaultBacklogSize is how many recent messages BeanBot reads as context.
const defaultBacklogSize = 50

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	backstory := os.Getenv("BEANBOT_BACKSTORY")
	if backstory == "" {
		log.Fatal("BEANBOT_BACKSTORY environment variable is not set")
	}

	location, err := serverTimezone()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	prompter, err := gemini.NewPrompter(ctx, backstory)
	if err != nil {
		log.Fatal(err)
	}

	bb, err := beanbot.NewBot(ctx, prompter, beanbot.Config{
		BacklogSize: backlogSize(),
		Location:    location,
	})
	if err != nil {
		log.Fatal(err)
	}

	bb.SetStatus("BeanBot")

	if err := bb.Start(); err != nil {
		log.Fatal(err)
	}
	log.Println("BeanBot has exited.")
}

// serverTimezone resolves the single timezone every relative time in a Trigger
// is interpreted against. BeanBot has no per-member timezone.
func serverTimezone() (*time.Location, error) {
	name := os.Getenv("BEANBOT_TIMEZONE")
	if name == "" {
		name = "UTC"
	}
	return time.LoadLocation(name)
}

func backlogSize() int {
	n, err := strconv.Atoi(os.Getenv("BEANBOT_BACKLOG_SIZE"))
	if err != nil || n <= 0 {
		return defaultBacklogSize
	}
	return n
}
