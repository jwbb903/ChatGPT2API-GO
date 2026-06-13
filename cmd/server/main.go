package main

import (
	"log"
	"net/http"
	"os"

	"chatgpt2api-go/internal/app"
)

func main() {
	srv, err := app.NewServer(".")
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("CHATGPT2API_ADDR")
	if addr == "" {
		addr = ":3000"
	}
	log.Printf("chatgpt2api-go listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
