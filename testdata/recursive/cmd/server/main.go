package main

import (
	"log"

	"github.com/shhac/grotto/testdata/recursive"
)

func main() {
	if err := recursive.StartServer(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
