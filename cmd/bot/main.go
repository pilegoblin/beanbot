package main

import (
	"context"
	"errors"
	"fmt"
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

// defaultMemoryLimit is how large a Guild's Memory may grow before Compaction
// rewrites it smaller. Every Trigger in that Guild carries the whole document,
// so this is a per-message cost as much as a disk one.
const defaultMemoryLimit = 8 << 10

// envFile holds configuration during local development. In production the
// process is configured with real environment variables instead.
const envFile = ".env"

func main() {
	if err := loadEnvFile(); err != nil {
		log.Fatal(err)
	}

	backstory := os.Getenv("BEANBOT_BACKSTORY")
	if backstory == "" {
		log.Fatal("BEANBOT_BACKSTORY environment variable is not set")
	}

	location, err := configuredTimezone()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	prompter, err := gemini.NewPrompter(ctx, backstory)
	if err != nil {
		log.Fatal(err)
	}

	// Fatal rather than degraded: an unset BEANBOT_MEMORY_DIR is the feature
	// switched off, but a configured directory that is not there means the
	// volume did not mount, and carrying on would write Memory to a filesystem
	// the next deploy discards.
	memory, err := beanbot.OpenMemory(os.Getenv("BEANBOT_MEMORY_DIR"), memoryLimit(), prompter)
	if err != nil {
		log.Fatal(err)
	}

	bb, err := beanbot.NewBot(ctx, prompter, beanbot.Config{
		BacklogSize: backlogSize(),
		Location:    location,
		Memory:      memory,
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

// loadEnvFile reads .env when it is there. Deployed builds have no such file —
// the environment is populated from fly secrets — so its absence is normal and
// not an error. A .env that exists but cannot be parsed still is, since
// ignoring it would silently run with the wrong configuration.
func loadEnvFile() error {
	if _, err := os.Stat(envFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("checking for %s: %w", envFile, err)
	}

	if err := godotenv.Load(envFile); err != nil {
		return fmt.Errorf("loading %s: %w", envFile, err)
	}
	return nil
}

// configuredTimezone resolves the single timezone every relative time in a
// Trigger is interpreted against. BeanBot has no per-member timezone, and no
// per-Guild one either — it is one setting for the whole deployment.
func configuredTimezone() (*time.Location, error) {
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

func memoryLimit() int {
	n, err := strconv.Atoi(os.Getenv("BEANBOT_MEMORY_LIMIT"))
	if err != nil || n <= 0 {
		return defaultMemoryLimit
	}
	return n
}
