package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// The LLM layer is OPTIONAL and provider-agnostic. It speaks the OpenAI-
// compatible /chat/completions API, so it works with Groq (free Llama tier —
// the default), Google Gemini, OpenRouter, Together, or OpenAI itself, just by
// setting env vars. With no LLM_API_KEY the whole app still runs on the
// deterministic keyword pipeline — the LLM only ever *upgrades* results.
//
//	LLM_API_KEY   — provider key (required to enable the LLM)
//	LLM_BASE_URL  — default https://api.groq.com/openai/v1
//	LLM_MODEL     — default llama-3.3-70b-versatile

func llmEnabled() bool { return os.Getenv("LLM_API_KEY") != "" }

func llmBaseURL() string {
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.groq.com/openai/v1"
}

func llmModel() string {
	if v := os.Getenv("LLM_MODEL"); v != "" {
		return v
	}
	return "llama-3.3-70b-versatile"
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type respFormat struct {
	Type string `json:"type"` // "json_object"
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// llmChat sends a system+user prompt and returns the assistant text. If
// wantJSON is set, providers that support it are asked for a JSON object.
func llmChat(system, user string, wantJSON bool) (string, error) {
	reqBody := chatRequest{
		Model:       llmModel(),
		Temperature: 0.2,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	if wantJSON {
		reqBody.ResponseFormat = &respFormat{Type: "json_object"}
	}
	body, _ := json.Marshal(reqBody)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, llmBaseURL()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("LLM_API_KEY"))

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("llm error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices (status %d)", resp.StatusCode)
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}
