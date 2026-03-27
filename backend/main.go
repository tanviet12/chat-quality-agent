package main

import (
	"log"

	"github.com/nmtan2001/chat-quality-agent/api"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/engine"
	"github.com/nmtan2001/chat-quality-agent/guesty"
)

var version = "dev"

func main() {
	log.Printf("Chat Quality Agent %s", version)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize Guesty client if credentials are provided
	if cfg.GuestyClientID != "" && cfg.GuestyClientSecret != "" {
		guesty.InitGlobalClient(cfg.GuestyClientID, cfg.GuestyClientSecret)
		log.Printf("Guesty API client initialized")
	}

	// Initialize JWT
	middleware.SetJWTSecret(cfg.JWTSecret)

	// Connect database
	if err := db.Connect(cfg.DSN(), cfg.IsProduction()); err != nil {
		log.Fatalf("Failed to connect database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Start scheduler
	scheduler, err := engine.NewScheduler(cfg)
	if err != nil {
		log.Fatalf("Failed to create scheduler: %v", err)
	}
	scheduler.Start()
	defer scheduler.Stop()

	// Setup router
	router := api.SetupRouter(cfg)

	// Start server
	log.Printf("CQA server starting on %s (env: %s)", cfg.ListenAddr(), cfg.Env)
	if err := router.Run(cfg.ListenAddr()); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
