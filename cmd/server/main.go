package main

import (
	"log"

	"pr-review-service/internal/config"
	"pr-review-service/internal/database"
	"pr-review-service/internal/handlers"
	"pr-review-service/internal/server"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting PR Review Service...")
	log.Printf("Database: %s:%s/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)
	log.Printf("Server port: %s", cfg.Port)

	db, err := database.New(cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	h := handlers.New(db)

	srv := server.New(h)

	if err := srv.Start(cfg.Port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}

	log.Printf("Server listening on port %s", cfg.Port)
}
