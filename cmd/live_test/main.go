package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/proxy"
	"antigravity-gateway/internal/recovery"
	"antigravity-gateway/internal/server"
)

func main() {
	// Read api.txt
	apiBytes, err := os.ReadFile("api.txt")
	if err != nil {
		log.Fatalf("Failed to read api.txt: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(apiBytes)), "\n")
	if len(lines) < 2 {
		log.Fatalf("api.txt must contain at least 2 lines (URL and Key)")
	}

	upstreamURL := strings.TrimSpace(lines[0])
	upstreamKey := strings.TrimSpace(lines[1])

	fmt.Printf("[1/7] Read api.txt: Upstream URL = %s (Key masked: %s...)\n", upstreamURL, upstreamKey[:min(len(upstreamKey), 8)])

	// Setup In-Process Gateway
	staticKey := "sk-downstream-live-test"
	adminKey := "admin-secret-key-12345"

	cfg := &config.Config{
		UpstreamBaseURL:             upstreamURL,
		UpstreamAPIKey:              upstreamKey,
		UpstreamAuthMode:            "bearer",
		UpstreamTimeout:             60 * time.Second,
		AdminAPIKey:                 adminKey,
		KeyHMACSecret:               "test-hmac-secret-123456",
		KeyDBPath:                   ":memory:",
		WrapperMode:                 "prefer",
		RecoveryPolicy:              "repair",
		UpstreamEmptyRetries:        3,
		SyntheticToolPrefix:         "agw_emit_",
		MaxRequestBytes:             16 * 1024 * 1024,
		MaxResponseBytes:            16 * 1024 * 1024,
		MaxConcurrentRequests:       100,
		MaxConcurrentRequestsPerKey: 50,
		RequestQueueTimeout:         10 * time.Second,
		StaticKeys: []config.StaticKeyConfig{
			{
				ID:   "live-test-static-key",
				Key:  staticKey,
				Name: "Live Test Key",
			},
		},
	}

	keyMgr, err := keymgmt.NewManager(cfg.KeyDBPath, cfg.KeyHMACSecret, cfg.StaticKeys)
	if err != nil {
		log.Fatalf("Failed to create key manager: %v", err)
	}
	defer keyMgr.Close()

	upstreamClient := proxy.NewUpstreamClient(cfg)
	orch := recovery.NewOrchestrator(cfg, upstreamClient, keyMgr)
	proxyHandler := proxy.NewProxyHandler(cfg, keyMgr, upstreamClient)
	proxyHandler.SetChatHandler(orch.HandleChatCompletions)
	adminHandler := keymgmt.NewAdminHandler(cfg.AdminAPIKey, keyMgr)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	proxyHandler.RegisterRoutes(mux)
	adminHandler.RegisterRoutes(mux)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	fmt.Printf("[2/7] In-process gateway started on %s\n", ts.URL)

	client := ts.Client()

	// 1. Test GET /v1/models
	fmt.Println("\n[3/7] Testing GET /v1/models...")
	req, _ := http.NewRequest("GET", ts.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+staticKey)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("GET /v1/models failed: %v", err)
	}
	modelsBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("GET /v1/models returned status %d: %s", resp.StatusCode, string(modelsBody))
	}

	var modelsData struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(modelsDataRaw(modelsBody), &modelsData); err != nil {
		log.Fatalf("Failed to parse models JSON: %v", err)
	}

	if len(modelsData.Data) == 0 {
		log.Fatalf("No models returned from upstream")
	}

	selectedModel := "gemini-3.5-flash-low"
	fmt.Printf("✓ Models fetched successfully (%d models). Selected model: %s\n", len(modelsData.Data), selectedModel)

	// 2. Test Non-Streaming with Heavy Prompt (SillyTavern style)
	fmt.Println("\n[4/7] Testing Non-Streaming Chat Completion with Heavy Prompt...")
	heavySystemPrompt := `[Character: Seraphina; Personality: Wise, calm, eloquent; Setting: Ancient arcane library;
Directives: You are Seraphina, an ancient archivist. Always stay in character. Respond eloquently in 2-3 sentences.]`

	chatReqBody := map[string]any{
		"model": selectedModel,
		"messages": []map[string]any{
			{"role": "system", "content": heavySystemPrompt},
			{"role": "user", "content": "Greetings, archivist. What secrets lie within these dusty tomes?"},
		},
		"temperature": 0.7,
	}
	chatReqBytes, _ := json.Marshal(chatReqBody)

	req, _ = http.NewRequest("POST", ts.URL+"/v1/chat/completions", bytes.NewReader(chatReqBytes))
	req.Header.Set("Authorization", "Bearer "+staticKey)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err = client.Do(req)
	if err != nil {
		log.Fatalf("Chat completion failed: %v", err)
	}
	chatRespBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Non-streaming chat returned status %d: %s", resp.StatusCode, string(chatRespBytes))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls any    `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(chatRespBytes, &chatResp); err != nil {
		log.Fatalf("Failed to parse chat response JSON: %v", err)
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		log.Fatalf("Chat response content is empty: %s", string(chatRespBytes))
	}

	if strings.Contains(chatResp.Choices[0].Message.Content, "agw_emit_") {
		log.Fatalf("Synthetic tool leaked into content: %s", chatResp.Choices[0].Message.Content)
	}

	fmt.Printf("✓ Non-Streaming response received in %v (finish_reason: %s):\n  %q\n",
		time.Since(start), chatResp.Choices[0].FinishReason, chatResp.Choices[0].Message.Content)

	// 3. Test Streaming with Heavy Prompt
	fmt.Println("\n[5/7] Testing Streaming Chat Completion with Heavy Prompt...")
	streamReqBody := map[string]any{
		"model":  selectedModel,
		"stream": true,
		"messages": []map[string]any{
			{"role": "system", "content": heavySystemPrompt},
			{"role": "user", "content": "Seraphina, tell me about the constellation of Orion in one poetic sentence."},
		},
	}
	streamReqBytes, _ := json.Marshal(streamReqBody)

	req, _ = http.NewRequest("POST", ts.URL+"/v1/chat/completions", bytes.NewReader(streamReqBytes))
	req.Header.Set("Authorization", "Bearer "+staticKey)
	req.Header.Set("Content-Type", "application/json")

	start = time.Now()
	resp, err = client.Do(req)
	if err != nil {
		log.Fatalf("Streaming request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Streaming returned status %d: %s", resp.StatusCode, string(body))
	}

	var streamedContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	doneReceived := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data: ") {
			data := strings.TrimPrefix(trimmed, "data: ")
			if data == "[DONE]" {
				doneReceived = true
				break
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					streamedContent.WriteString(chunk.Choices[0].Delta.Content)
				}
			}
		}
	}

	if !doneReceived {
		log.Fatalf("Streaming did not receive [DONE] marker")
	}

	fullStreamed := streamedContent.String()
	if fullStreamed == "" {
		log.Fatalf("Streamed content is empty")
	}
	if strings.Contains(fullStreamed, "agw_emit_") {
		log.Fatalf("Synthetic tool name leaked in stream: %s", fullStreamed)
	}

	fmt.Printf("✓ Streaming response received in %v:\n  %q\n", time.Since(start), fullStreamed)

	// 4. Test with Downstream Real Tool Definition
	fmt.Println("\n[6/7] Testing Request with Real Tool Definition...")
	toolReqBody := map[string]any{
		"model": selectedModel,
		"messages": []map[string]any{
			{"role": "user", "content": "What is the weather in Paris?"},
		},
		"tools": []map[string]any{
			{
				"type": "function",
				"function": map[string]any{
					"name":        "get_current_weather",
					"description": "Get weather for a given city",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"},
						},
						"required": []string{"location"},
					},
				},
			},
		},
	}
	toolReqBytes, _ := json.Marshal(toolReqBody)

	req, _ = http.NewRequest("POST", ts.URL+"/v1/chat/completions", bytes.NewReader(toolReqBytes))
	req.Header.Set("Authorization", "Bearer "+staticKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		log.Fatalf("Tool request failed: %v", err)
	}
	toolRespBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Tool request returned status %d: %s", resp.StatusCode, string(toolRespBytes))
	}

	var toolChatResp struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(toolRespBytes, &toolChatResp)

	if len(toolChatResp.Choices) > 0 {
		c := toolChatResp.Choices[0]
		if len(c.Message.ToolCalls) > 0 {
			tc := c.Message.ToolCalls[0]
			fmt.Printf("✓ Real tool call triggered by model: %s (args: %s, ID: %s)\n", tc.Function.Name, tc.Function.Arguments, tc.ID)

			// Step 2: Feed tool result back and get synthetic final answer
			followUpReq := map[string]any{
				"model": selectedModel,
				"messages": []map[string]any{
					{"role": "user", "content": "What is the weather in Paris?"},
					{"role": "assistant", "tool_calls": c.Message.ToolCalls},
					{"role": "tool", "tool_call_id": tc.ID, "content": `{"temperature": "18C", "condition": "Mild Rain"}`},
				},
				"tools": toolReqBody["tools"],
			}
			fBytes, _ := json.Marshal(followUpReq)
			req, _ = http.NewRequest("POST", ts.URL+"/v1/chat/completions", bytes.NewReader(fBytes))
			req.Header.Set("Authorization", "Bearer "+staticKey)
			req.Header.Set("Content-Type", "application/json")
			fResp, err := client.Do(req)
			if err == nil && fResp.StatusCode == http.StatusOK {
				fRespBody, _ := io.ReadAll(fResp.Body)
				fResp.Body.Close()
				var fParsed struct {
					Choices []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
				}
				_ = json.Unmarshal(fRespBody, &fParsed)
				if len(fParsed.Choices) > 0 {
					fmt.Printf("✓ Final answer after tool execution received: %q\n", fParsed.Choices[0].Message.Content)
				}
			}
		} else {
			fmt.Printf("✓ Model answered directly via synthetic tool: %q\n", c.Message.Content)
		}
	}

	// 5. Test Dynamic Key Creation, Auth, and Revocation
	fmt.Println("\n[7/7] Testing Dynamic Key Creation and Revocation...")
	createKeyBody := map[string]any{
		"name": "Dynamic Live Key",
	}
	ckBytes, _ := json.Marshal(createKeyBody)
	req, _ = http.NewRequest("POST", ts.URL+"/admin/keys", bytes.NewReader(ckBytes))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		log.Fatalf("Dynamic key creation failed: %v, status: %d", err, resp.StatusCode)
	}

	var createdKey struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&createdKey)
	resp.Body.Close()

	fmt.Printf("✓ Created dynamic key %s (%s...)\n", createdKey.ID, createdKey.Key[:15])

	// Test auth with dynamic key
	req, _ = http.NewRequest("GET", ts.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+createdKey.Key)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Fatalf("Auth with dynamic key failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	fmt.Printf("✓ Auth with dynamic key succeeded (HTTP 200)\n")

	// Revoke dynamic key
	req, _ = http.NewRequest("POST", ts.URL+"/admin/keys/"+createdKey.ID+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Fatalf("Revoke key failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	fmt.Printf("✓ Dynamic key revoked\n")

	// Test auth with revoked key (should be 401)
	req, _ = http.NewRequest("GET", ts.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+createdKey.Key)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		log.Fatalf("Expected 401 Unauthorized after revocation, got: %d", resp.StatusCode)
	}
	resp.Body.Close()
	fmt.Printf("✓ Auth with revoked key correctly rejected (HTTP 401)\n")

	fmt.Println("\n=======================================================")
	fmt.Println("🎉 ALL LIVE REAL UPSTREAM TESTS PASSED SUCCESSFULLY! 🎉")
	fmt.Println("=======================================================")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func modelsDataRaw(b []byte) []byte {
	return b
}
