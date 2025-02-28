package main

import (
	"context"
	"log"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	bb, err := NewBot(context.Background())
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
