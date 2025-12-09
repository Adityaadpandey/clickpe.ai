package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
)

type TriggerEvent struct {
	UserCount int    `json:"user_count"`
	Timestamp string `json:"timestamp"`
}

type Response struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

func handler(ctx context.Context, event TriggerEvent) (Response, error) {
	webhookURL := os.Getenv("N8N_WEBHOOK_URL")
	if webhookURL == "" {
		return Response{}, fmt.Errorf("N8N_WEBHOOK_URL not set")
	}

	log.Printf("Triggering n8n workflow for %d users", event.UserCount)

	// Prepare payload for n8n
	payload := map[string]interface{}{
		"trigger":    "csv_processed",
		"user_count": event.UserCount,
		"timestamp":  event.Timestamp,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Make HTTP POST request to n8n webhook
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return Response{}, fmt.Errorf("failed to call n8n webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("n8n webhook returned error status: %d", resp.StatusCode)
	}

	log.Printf("Successfully triggered n8n workflow, status: %d", resp.StatusCode)

	return Response{
		Message: fmt.Sprintf("Matching workflow triggered for %d users", event.UserCount),
		Status:  "success",
	}, nil
}

func main() {
	lambda.Start(handler)
}
