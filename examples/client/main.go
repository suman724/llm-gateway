package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Standard Chat Completion Request (OpenAI Compatible)
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func main() {
	gatewayURL := "http://localhost:8080/v1/chat/completions"
	apiKey := "my-tenant-key" // Matches a key in your DynamoDB Tenants table

	reqBody := ChatRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Tell me a short story about a robot."},
		},
		Stream: true, // Demonstrate Streaming
	}

	jsonData, _ := json.Marshal(reqBody)

	fmt.Printf("Sending request to %s...\n", gatewayURL)
	req, _ := http.NewRequest("POST", gatewayURL, bytes.NewBuffer(jsonData))

	// Standard Headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Gateway-Specific Resilience Headers (Optional)
	req.Header.Set("X-LLM-Retry-Max", "5")
	req.Header.Set("X-LLM-Retry-Backoff-Ms", "500")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error Body: %s\n", string(body))
		os.Exit(1)
	}

	// Handle Streaming Response
	fmt.Println("\n--- Response Stream ---")
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// In a real client, you would parse "data: {...}" here
		// We just print the raw SSE line to prove connectivity
		if strings.HasPrefix(line, "data:") {
			fmt.Println(line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Stream error: %v\n", err)
	}
	fmt.Println("\n--- End of Stream ---")
}
