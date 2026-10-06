package main

import (
	"log"
	"os"

	"github.com/devfiqi/personal-search/core/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}
