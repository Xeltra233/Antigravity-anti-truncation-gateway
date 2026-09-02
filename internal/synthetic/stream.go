package synthetic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"antigravity-gateway/internal/config"
)

type StreamStats struct {
	SyntheticHit       bool
	RealToolCallCount  int
	SyntheticCallCount int
	ContentConflict    bool
	BytesWritten       int64
}

type ChoiceStreamState struct {
	Index              int
	HasSynthetic       bool
	SyntheticDecoder   *IncrementalJSONStringDecoder
	SyntheticToolIndex int
	SideBuffer         strings.Builder
	SideBufferEmitted  bool
	RealToolsSeen      map[int]int // original index -> remapped index
	NextRealIndex      int
	RoleEmitted        bool
}

type StreamTransformer struct {
	cfg               *config.Config
	syntheticToolName string
}

func NewStreamTransformer(cfg *config.Config, syntheticToolName string) *StreamTransformer {
	return &StreamTransformer{
		cfg:               cfg,
		syntheticToolName: syntheticToolName,
	}
}

type RawChunk struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []RawChunkChoice `json:"choices"`
	Usage   any              `json:"usage,omitempty"`
}

type RawChunkChoice struct {
	Index        int           `json:"index"`
	Delta        RawChunkDelta `json:"delta"`
	FinishReason any           `json:"finish_reason"`
}

type RawChunkDelta struct {
	Role             string             `json:"role,omitempty"`
	Content          any                `json:"content,omitempty"` // string or nil
	ReasoningContent any                `json:"reasoning_content,omitempty"`
	Reasoning        any                `json:"reasoning,omitempty"`
	ToolCalls        []RawChunkToolCall `json:"tool_calls,omitempty"`
}

type RawChunkToolCall struct {
	Index    int                      `json:"index"`
	ID       string                   `json:"id,omitempty"`
	Type     string                   `json:"type,omitempty"`
	Function RawChunkFunctionCallDesc `json:"function"`
}

type RawChunkFunctionCallDesc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func (st *StreamTransformer) Transform(upstreamReader io.Reader, w io.Writer, flusher http.Flusher) (*StreamStats, error) {
	stats := &StreamStats{}
	scanner := bufio.NewScanner(upstreamReader)
	// Buffer size up to max response chunk
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, int(st.cfg.MaxResponseBytes))

	states := make(map[int]*ChoiceStreamState)

	getState := func(idx int) *ChoiceStreamState {
		if s, ok := states[idx]; ok {
			return s
		}
		s := &ChoiceStreamState{
			Index:              idx,
			SyntheticDecoder:   NewIncrementalJSONStringDecoder(),
			SyntheticToolIndex: -1,
			RealToolsSeen:      make(map[int]int),
		}
		states[idx] = s
		return s
	}

	writeSSEData := func(data any) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("data: %s\n\n", string(b))
		n, err := io.WriteString(w, msg)
		stats.BytesWritten += int64(n)
		if flusher != nil {
			flusher.Flush()
		}
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, ":") {
			// SSE comment/ping, forward or ignore
			continue
		}

		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}

		dataContent := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if dataContent == "[DONE]" {
			break
		}

		var chunk RawChunk
		if err := json.Unmarshal([]byte(dataContent), &chunk); err != nil {
			// Raw unparseable chunk, ignore or log
			continue
		}

		// If chunk has no choices but has usage info, forward as usage chunk
		if len(chunk.Choices) == 0 && chunk.Usage != nil {
			usageChunk := map[string]any{
				"id":      chunk.ID,
				"object":  "chat.completion.chunk",
				"created": chunk.Created,
				"model":   chunk.Model,
				"choices": []any{},
				"usage":   chunk.Usage,
			}
			_ = writeSSEData(usageChunk)
			continue
		}

		// Process choices
		for _, c := range chunk.Choices {
			state := getState(c.Index)

			// Emit initial role chunk if present
			if c.Delta.Role != "" && !state.RoleEmitted {
				roleChunk := map[string]any{
					"id":      chunk.ID,
					"object":  "chat.completion.chunk",
					"created": chunk.Created,
					"model":   chunk.Model,
					"choices": []map[string]any{
						{
							"index": c.Index,
							"delta": map[string]any{
								"role": c.Delta.Role,
							},
							"finish_reason": nil,
						},
					},
				}
				_ = writeSSEData(roleChunk)
				state.RoleEmitted = true
			}

			// Process reasoning content delta (e.g. thinking models)
			var reasoningStr string
			if c.Delta.ReasoningContent != nil {
				if s, ok := c.Delta.ReasoningContent.(string); ok && s != "" {
					reasoningStr = s
				}
			} else if c.Delta.Reasoning != nil {
				if s, ok := c.Delta.Reasoning.(string); ok && s != "" {
					reasoningStr = s
				}
			}
			if reasoningStr != "" {
				reasoningChunk := map[string]any{
					"id":      chunk.ID,
					"object":  "chat.completion.chunk",
					"created": chunk.Created,
					"model":   chunk.Model,
					"choices": []map[string]any{
						{
							"index": c.Index,
							"delta": map[string]any{
								"reasoning_content": reasoningStr,
							},
							"finish_reason": nil,
						},
					},
				}
				_ = writeSSEData(reasoningChunk)
			}

			// Process tool calls
			if len(c.Delta.ToolCalls) > 0 {
				var realToolCallsToEmit []map[string]any

				for _, tc := range c.Delta.ToolCalls {
					tIdx := tc.Index

					// Check if this tool index is synthetic
					if state.SyntheticToolIndex == tIdx || (tc.Function.Name != "" && tc.Function.Name == st.syntheticToolName) {
						state.HasSynthetic = true
						state.SyntheticToolIndex = tIdx
						stats.SyntheticHit = true

						// Discard side buffer on synthetic hit
						if state.SideBuffer.Len() > 0 && !state.SideBufferEmitted {
							stats.ContentConflict = true
							state.SideBuffer.Reset()
						} else if state.SideBufferEmitted {
							stats.ContentConflict = true
						}

						if tc.Function.Arguments != "" {
							emitted, _, err := state.SyntheticDecoder.Feed(tc.Function.Arguments)
							if err == nil && emitted != "" {
								contentChunk := map[string]any{
									"id":      chunk.ID,
									"object":  "chat.completion.chunk",
									"created": chunk.Created,
									"model":   chunk.Model,
									"choices": []map[string]any{
										{
											"index": c.Index,
											"delta": map[string]any{
												"content": emitted,
											},
											"finish_reason": nil,
										},
									},
								}
								_ = writeSSEData(contentChunk)
							}
						}
					} else {
						// Real tool call
						stats.RealToolCallCount++
						remappedIdx, exists := state.RealToolsSeen[tIdx]
						if !exists {
							remappedIdx = state.NextRealIndex
							state.RealToolsSeen[tIdx] = remappedIdx
							state.NextRealIndex++
						}

						fnMap := map[string]any{}
						if tc.Function.Name != "" {
							fnMap["name"] = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							fnMap["arguments"] = tc.Function.Arguments
						}

						tcMap := map[string]any{
							"index":    remappedIdx,
							"function": fnMap,
						}
						if tc.ID != "" {
							tcMap["id"] = tc.ID
						}
						if tc.Type != "" {
							tcMap["type"] = tc.Type
						}

						realToolCallsToEmit = append(realToolCallsToEmit, tcMap)
					}
				}

				if len(realToolCallsToEmit) > 0 {
					toolChunk := map[string]any{
						"id":      chunk.ID,
						"object":  "chat.completion.chunk",
						"created": chunk.Created,
						"model":   chunk.Model,
						"choices": []map[string]any{
							{
								"index": c.Index,
								"delta": map[string]any{
									"tool_calls": realToolCallsToEmit,
								},
								"finish_reason": nil,
							},
						},
					}
					_ = writeSSEData(toolChunk)
				}
			}

			// Process standard content delta with true streaming
			if c.Delta.Content != nil {
				if contentStr, ok := c.Delta.Content.(string); ok && contentStr != "" {
					if state.HasSynthetic {
						// Drop duplicate standard content when synthetic tool is already active
						stats.ContentConflict = true
					} else {
						if st.cfg.StreamSideBufferBytes > 0 && !state.SideBufferEmitted {
							state.SideBuffer.WriteString(contentStr)
							if state.SideBuffer.Len() >= st.cfg.StreamSideBufferBytes {
								flushChunk := map[string]any{
									"id":      chunk.ID,
									"object":  "chat.completion.chunk",
									"created": chunk.Created,
									"model":   chunk.Model,
									"choices": []map[string]any{
										{
											"index": c.Index,
											"delta": map[string]any{
												"content": state.SideBuffer.String(),
											},
											"finish_reason": nil,
										},
									},
								}
								_ = writeSSEData(flushChunk)
								state.SideBufferEmitted = true
								state.SideBuffer.Reset()
							}
						} else {
							// True streaming: emit immediately!
							if state.SideBuffer.Len() > 0 {
								contentStr = state.SideBuffer.String() + contentStr
								state.SideBuffer.Reset()
							}
							contentChunk := map[string]any{
								"id":      chunk.ID,
								"object":  "chat.completion.chunk",
								"created": chunk.Created,
								"model":   chunk.Model,
								"choices": []map[string]any{
									{
										"index": c.Index,
										"delta": map[string]any{
											"content": contentStr,
										},
										"finish_reason": nil,
									},
								},
							}
							_ = writeSSEData(contentChunk)
							state.SideBufferEmitted = true
						}
					}
				}
			}

			// Process finish_reason
			if c.FinishReason != nil {
				finishReasonStr, _ := c.FinishReason.(string)

				// If choice had synthetic call, check if remaining finish reason was tool_calls
				if state.HasSynthetic {
					// Check remaining buffer repair
					rem, _ := state.SyntheticDecoder.Finish(int(st.cfg.MaxResponseBytes))
					if rem != "" {
						remChunk := map[string]any{
							"id":      chunk.ID,
							"object":  "chat.completion.chunk",
							"created": chunk.Created,
							"model":   chunk.Model,
							"choices": []map[string]any{
								{
									"index": c.Index,
									"delta": map[string]any{
										"content": rem,
									},
									"finish_reason": nil,
								},
							},
						}
						_ = writeSSEData(remChunk)
					}

					if finishReasonStr == "tool_calls" {
						if len(state.RealToolsSeen) == 0 {
							finishReasonStr = "stop"
						}
					}
				} else {
					// No synthetic call: flush remaining side buffer if any
					if state.SideBuffer.Len() > 0 {
						fbChunk := map[string]any{
							"id":      chunk.ID,
							"object":  "chat.completion.chunk",
							"created": chunk.Created,
							"model":   chunk.Model,
							"choices": []map[string]any{
								{
									"index": c.Index,
									"delta": map[string]any{
										"content": state.SideBuffer.String(),
									},
									"finish_reason": nil,
								},
							},
						}
						_ = writeSSEData(fbChunk)
						state.SideBufferEmitted = true
						state.SideBuffer.Reset()
					}
				}
				finalChunk := map[string]any{
					"id":      chunk.ID,
					"object":  "chat.completion.chunk",
					"created": chunk.Created,
					"model":   chunk.Model,
					"choices": []map[string]any{
						{
							"index":         c.Index,
							"delta":         map[string]any{},
							"finish_reason": finishReasonStr,
						},
					},
				}
				if chunk.Usage != nil {
					finalChunk["usage"] = chunk.Usage
				}
				_ = writeSSEData(finalChunk)
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return stats, fmt.Errorf("error reading upstream SSE stream: %w", err)
	}

	// Emit [DONE]
	doneMsg := "data: [DONE]\n\n"
	n, _ := io.WriteString(w, doneMsg)
	stats.BytesWritten += int64(n)
	if flusher != nil {
		flusher.Flush()
	}

	return stats, nil
}
