package main

import (
	"log"
	"os"

	"github.com/devfiqi/personal-search/core/internal/app"
)

func main() {
	if err := app.Run(os.Stdout); err != nil {
		log.Fatal(err)
	}
}
