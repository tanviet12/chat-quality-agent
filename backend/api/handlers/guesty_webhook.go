package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GuestyWebhook handles incoming webhooks from Guesty
func GuestyWebhook(c *gin.Context) {
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Printf("[Guesty Webhook] Failed to parse payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	event, _ := payload["event"].(string)
	log.Printf("[Guesty Webhook] Received event: %s", event)
	log.Printf("[Guesty Webhook] Full payload: %+v", payload)

	// For message events
	if event == "reservation.messageReceived" || event == "reservation.messageSent" {
		message, _ := payload["message"].(map[string]interface{})
		conversation, _ := payload["conversation"].(map[string]interface{})

		messageBody, _ := message["body"].(string)
		guestName := ""
		if meta, ok := conversation["meta"].(map[string]interface{}); ok {
			guestName, _ = meta["guestName"].(string)
		}

		log.Printf("[Guesty Webhook] Message from %s: %s", guestName, messageBody)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// GuestyWebhookChallenge handles Guesty webhook verification
func GuestyWebhookChallenge(c *gin.Context) {
	// Guesty may send a verification challenge
	challenge := c.Query("challenge")
	if challenge != "" {
		log.Printf("[Guesty Webhook] Verification challenge received")
		c.JSON(http.StatusOK, gin.H{"challenge": challenge})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
