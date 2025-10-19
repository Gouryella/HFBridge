package main

import (
	"log"
	"net/http"

	"hfbridge/internal/config"
	"hfbridge/internal/logdb"
	"hfbridge/internal/server"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	store, err := logdb.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := server.ListenAndServe(cfg, store); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
