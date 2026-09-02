package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/limiter"
	"antigravity-gateway/internal/metrics"
	"antigravity-gateway/internal/proxy"
	"antigravity-gateway/internal/server"
	"antigravity-gateway/internal/synthetic"
)

type Orchestrator struct {
	cfg        *config.Config
	client     *proxy.UpstreamClient
	keyMgr     *keymgmt.Manager
	injector   *synthetic.RequestInjector
	normalizer *synthetic.ResponseNormalizer
	limiter    *limiter.ConcurrencyLimiter
}

func NewOrchestrator(cfg *config.Config, client *proxy.UpstreamClient, keyMgr *keymgmt.Manager) *Orchestrator {
	return &Orchestrator{
		cfg:        cfg,
		client:     client,
		keyMgr:     keyMgr,
		injector:   synthetic.NewRequestInjector(cfg),
		normalizer: synthetic.NewResponseNormalizer(cfg),
		limiter:    limiter.NewConcurrencyLimiter(cfg.MaxConcurrentRequests, cfg.MaxConcurrentRequestsPerKey, cfg.RequestQueueTimeout),
	}
}

func (o *Orchestrator) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	metrics.Default.RequestsTotal.Add(1)
	metrics.Default.ActiveRequests.Add(1)
	defer metrics.Default.ActiveRequests.Add(-1)

	reqID := proxy.GetRequestID(r.Context())
	keyInfo := proxy.GetKeyInfo(r.Context())

	// Acquire concurrency token
	keyID := ""
	if keyInfo != nil {
		keyID = keyInfo.ID
	}
	release, err := o.limiter.Acquire(r.Context(), keyID)
	if err != nil {
		metrics.Default.OverloadRejectionsTotal.Add(1)
		w.Header().Set("Retry-After", "1")
		if errors.Is(err, limiter.ErrKeyLimitExceeded) {
			server.WriteError(w, http.StatusTooManyRequests, "Concurrent request limit exceeded for this API key", "rate_limit_error", "rate_limit_exceeded")
			return
		}
		if errors.Is(err, limiter.ErrGlobalLimitExceeded) {
			server.WriteError(w, http.StatusServiceUnavailable, "Server is overloaded, please retry shortly", "server_error", "server_overloaded")
			return
		}
		return
	}
	defer release()

	// Read downstream request body
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, o.cfg.MaxRequestBytes))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}

	// Extract requested model
	var partialReq struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(bodyBytes, &partialReq); err != nil {
		server.WriteError(w, http.StatusBadRequest, "Invalid JSON in request body", "invalid_request_error", "invalid_json")
		return
	}

	// Check model permission
	if !o.keyMgr.IsModelAllowed(keyInfo, partialReq.Model) {
		server.WriteError(w, http.StatusForbidden, fmt.Sprintf("Model %q is not permitted for this API key", partialReq.Model), "permission_denied", "model_forbidden")
		return
	}

	// Inject synthetic transport tool
	injected, err := o.injector.Inject(bodyBytes)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "Failed to prepare request: "+err.Error(), "api_error", "internal_error")
		return
	}

	if injected.IsStreaming {
		o.handleStreaming(w, r.Context(), injected, reqID)
	} else {
		o.handleNonStreaming(w, r.Context(), injected, reqID, bodyBytes)
	}
}

func (o *Orchestrator) handleNonStreaming(w http.ResponseWriter, ctx context.Context, injected *synthetic.InjectedRequest, reqID string, originalBody []byte) {
	maxAttempts := 1 + o.cfg.UpstreamEmptyRetries
	var lastRespBody []byte
	var lastRespStatus int
	var lastRespHeader http.Header

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return
		}

		currentReqID := reqID
		if attempt > 1 {
			currentReqID = fmt.Sprintf("%s_empty_retry_%d", reqID, attempt)
			metrics.Default.SyntheticRetries.Add(1)
		}

		upstreamReq, err := o.client.NewUpstreamRequest(ctx, http.MethodPost, o.cfg.UpstreamChatURL(), bytes.NewReader(injected.TransformedBody), currentReqID)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, "Failed to create upstream request", "api_error", "internal_error")
			return
		}

		resp, err := o.client.Do(upstreamReq)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			server.WriteError(w, http.StatusBadGateway, "Upstream connection failed: "+err.Error(), "api_error", "upstream_error")
			return
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, o.cfg.MaxResponseBytes))
		resp.Body.Close()
		if err != nil {
			server.WriteError(w, http.StatusBadGateway, "Failed to read upstream response", "api_error", "upstream_error")
			return
		}

		lastRespBody = respBody
		lastRespStatus = resp.StatusCode
		lastRespHeader = resp.Header

		if resp.StatusCode != http.StatusOK {
			// Non-200, return immediately
			w.Header().Set("Content-Type", lastRespHeader.Get("Content-Type"))
			w.WriteHeader(lastRespStatus)
			_, _ = w.Write(lastRespBody)
			return
		}

		// Check for empty response (空回)
		if isResponseEmpty(respBody) {
			if attempt < maxAttempts {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		// Found non-empty response or exhausted empty retries
		break
	}

	// Normalize response
	normalized, stats, normErr := o.normalizer.NormalizeNonStreaming(lastRespBody, injected.SyntheticToolName)
	if normErr != nil {
		// If decode failed and policy allows format retry
		if o.cfg.RecoveryPolicy == "repair_then_retry" && stats != nil && stats.RealToolCallCount == 0 && ctx.Err() == nil {
			retryNormalized, retryErr := o.attemptSingleFormatRetry(ctx, originalBody, reqID)
			if retryErr == nil {
				metrics.Default.SyntheticRetries.Add(1)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(retryNormalized)
				return
			}
		}

		server.WriteError(w, http.StatusBadGateway, "Synthetic tool decode failed: "+normErr.Error(), "gateway_protocol_error", "synthetic_tool_decode_failed")
		return
	}

	if stats != nil {
		if stats.SyntheticHit {
			metrics.Default.SyntheticHits.Add(1)
		}
		if stats.SyntheticRepaired {
			metrics.Default.SyntheticRepairs.Add(1)
		}
		if stats.ContentConflict {
			metrics.Default.SyntheticConflicts.Add(1)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(normalized)
}

func (o *Orchestrator) attemptSingleFormatRetry(ctx context.Context, originalBody []byte, reqID string) ([]byte, error) {
	// Prepare retry injection with error correction message
	var bodyMap map[string]any
	if err := json.Unmarshal(originalBody, &bodyMap); err != nil {
		return nil, err
	}

	newToolName, err := o.injector.GenerateUniqueToolName(nil)
	if err != nil {
		return nil, err
	}

	// Append synthetic tool
	syntheticTool := o.injector.BuildSyntheticTool(newToolName)
	var rawTools []any
	if tList, ok := bodyMap["tools"].([]any); ok {
		rawTools = append(rawTools, tList...)
	}
	rawTools = append(rawTools, syntheticTool)
	bodyMap["tools"] = rawTools
	bodyMap["tool_choice"] = map[string]any{
		"type":     "function",
		"function": map[string]any{"name": newToolName},
	}

	// Add correction message as user role to guarantee Gemini turn compatibility
	correctionMsg := map[string]any{
		"role":    "user",
		"content": fmt.Sprintf("Your previous response had formatting issues in the transport tool arguments. Please call the transport tool `%s` with valid JSON format `{\"content\": \"...\"}`.", newToolName),
	}
	var messages []any
	if mList, ok := bodyMap["messages"].([]any); ok {
		messages = append(messages, mList...)
	}
	messages = append(messages, correctionMsg)
	bodyMap["messages"] = messages

	retryBody, _ := json.Marshal(bodyMap)

	retryReq, err := o.client.NewUpstreamRequest(ctx, http.MethodPost, o.cfg.UpstreamChatURL(), bytes.NewReader(retryBody), reqID+"_format_retry")
	if err != nil {
		return nil, err
	}

	resp, err := o.client.Do(retryReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("retry returned upstream status %d", resp.StatusCode)
	}

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, o.cfg.MaxResponseBytes))
	if err != nil {
		return nil, err
	}

	normalized, _, err := o.normalizer.NormalizeNonStreaming(respBytes, newToolName)
	if err != nil {
		return nil, err
	}

	return normalized, nil
}

func (o *Orchestrator) handleStreaming(w http.ResponseWriter, ctx context.Context, injected *synthetic.InjectedRequest, reqID string) {
	upstreamReq, err := o.client.NewUpstreamRequest(ctx, http.MethodPost, o.cfg.UpstreamChatURL(), bytes.NewReader(injected.TransformedBody), reqID)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "Failed to create upstream request", "api_error", "internal_error")
		return
	}

	resp, err := o.client.Do(upstreamReq)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		server.WriteError(w, http.StatusBadGateway, "Upstream connection failed: "+err.Error(), "api_error", "upstream_error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	if injected.SyntheticToolName == "" {
		// Passthrough mode with immediate chunk-by-chunk flushing
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		return
	}

	transformer := synthetic.NewStreamTransformer(o.cfg, injected.SyntheticToolName)
	stats, _ := transformer.Transform(resp.Body, w, flusher)
	if stats != nil {
		if stats.SyntheticHit {
			metrics.Default.SyntheticHits.Add(1)
		}
		if stats.ContentConflict {
			metrics.Default.SyntheticConflicts.Add(1)
		}
	}
}

func isResponseEmpty(respBody []byte) bool {
	if len(respBody) == 0 {
		return true
	}
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return false
	}
	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		return true
	}
	for _, c := range choices {
		cMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		msg, ok := cMap["message"].(map[string]any)
		if !ok {
			continue
		}
		content, _ := msg["content"].(string)
		if strings.TrimSpace(content) != "" {
			return false
		}
		if tcList, ok := msg["tool_calls"].([]any); ok && len(tcList) > 0 {
			return false
		}
	}
	return true
}
