package main

import (
	"log"
	"os"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/lsp"
)

func main() {
	server := lsp.NewServer(os.Stdin, os.Stdout)
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
