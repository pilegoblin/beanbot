package main

import (
	"log"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	bb, err := NewBot()
	if err != nil {
		log.Fatal(err)
	}

	bb.SetStatus("BeanBot V2")

	err = bb.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("BeanBot has exited.")
}
