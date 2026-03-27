package guesty

import (
	"log"
	"sync"
)

var (
	globalClient *Client
	initOnce     sync.Once
)

// InitGlobalClient initializes the global Guesty client with credentials
func InitGlobalClient(clientID, clientSecret string) {
	initOnce.Do(func() {
		globalClient = NewClient(clientID, clientSecret)
		log.Printf("[Guesty] Global client initialized")
	})
}

// GlobalClient returns the global Guesty client instance
func GlobalClient() *Client {
	if globalClient == nil {
		log.Panic("[Guesty] Global client not initialized. Call InitGlobalClient first.")
	}
	return globalClient
}
