package imagectx

import (
	"encoding/json"
	"fmt"
	"strings"
)

// IsImageGenerationRequest returns true if the model or request represents an image generation/editing task.
func IsImageGenerationRequest(model string, rawPrompt string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "dall-e") ||
		strings.Contains(m, "flux") ||
		strings.Contains(m, "midjourney") ||
		strings.Contains(m, "stable-diffusion") ||
		strings.Contains(m, "imagen") ||
		strings.HasSuffix(m, "-image") ||
		strings.Contains(m, "flash-image") ||
		strings.Contains(m, "image-gen") {
		return true
	}
	return false
}

// TransformRequest transforms the JSON request body according to the given mode.
// Returns (transformedBody, fallbackTriggered, error).
func TransformRequest(bodyBytes []byte, cfg *PipelineConfig, targetMode Mode) ([]byte, bool, error) {
	if cfg == nil {
		cfg = DefaultPipelineConfig()
	}

	if targetMode == ModeStandard || targetMode == "" {
		return bodyBytes, false, nil
	}

	var rootMap map[string]any
	if err := json.Unmarshal(bodyBytes, &rootMap); err != nil {
		return bodyBytes, false, fmt.Errorf("invalid json in request: %w", err)
	}

	modelName, _ := rootMap["model"].(string)
	if IsImageGenerationRequest(modelName, "") {
		return bodyBytes, false, nil
	}

	rawMessages, ok := rootMap["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		return bodyBytes, false, nil
	}

	rasterizer, err := NewRasterizer(cfg)
	if err != nil {
		if cfg.FallbackOnError {
			return bodyBytes, true, nil
		}
		return bodyBytes, false, fmt.Errorf("failed to create rasterizer: %w", err)
	}
	chunker := NewChunker(cfg.MaxRunesPerPage, cfg.MaxLinesPerPage)

	var totalImagesCount int
	var totalImagesBytes int64

	// If ModeCurrentTurnOnly, find the index of the last assistant message.
	// All user messages after the last assistant turn belong to the current turn.
	lastAssistantIdx := -1
	if targetMode == ModeCurrentTurnOnly {
		for i := len(rawMessages) - 1; i >= 0; i-- {
			msgMap, ok := rawMessages[i].(map[string]any)
			if ok {
				role, _ := msgMap["role"].(string)
				if strings.ToLower(role) == "assistant" {
					lastAssistantIdx = i
					break
				}
			}
		}
	}

	newMessages := make([]any, 0, len(rawMessages))

	for msgIdx, item := range rawMessages {
		msgMap, ok := item.(map[string]any)
		if !ok {
			newMessages = append(newMessages, item)
			continue
		}

		role, _ := msgMap["role"].(string)
		roleLower := strings.ToLower(strings.TrimSpace(role))

		// 1. Tool / Function responses MUST remain native JSON/text to preserve function calling protocol
		if roleLower == "tool" || roleLower == "function" {
			newMessages = append(newMessages, msgMap)
			continue
		}

		// 2. If ModeCurrentTurnOnly, any message at or before the last assistant message (or non-user message) remains text
		if targetMode == ModeCurrentTurnOnly {
			if msgIdx <= lastAssistantIdx || roleLower != "user" {
				newMessages = append(newMessages, msgMap)
				continue
			}
		}

		// 3. Process message content for rasterization
		var newParts []any
		rawContent := msgMap["content"]

		switch cv := rawContent.(type) {
		case string:
			if strings.TrimSpace(cv) == "" {
				newParts = append(newParts, map[string]any{
					"type": "text",
					"text": cv,
				})
			} else {
				chunks := chunker.ChunkText(cv)
				for _, chk := range chunks {
					totalImagesCount++
					if cfg.MaxPages > 0 && totalImagesCount > cfg.MaxPages {
						if cfg.FallbackOnError {
							return bodyBytes, true, nil
						}
						return bodyBytes, false, fmt.Errorf("request exceeded max pages limit (%d)", cfg.MaxPages)
					}

					rendered, err := rasterizer.RenderChunk(roleLower, chk)
					if err != nil {
						if cfg.FallbackOnError {
							return bodyBytes, true, nil
						}
						return bodyBytes, false, fmt.Errorf("rasterization error on msg %d: %w", msgIdx, err)
					}

					totalImagesBytes += rendered.ByteLength
					if cfg.MaxTotalBytes > 0 && totalImagesBytes > cfg.MaxTotalBytes {
						if cfg.FallbackOnError {
							return bodyBytes, true, nil
						}
						return bodyBytes, false, fmt.Errorf("request exceeded total image bytes limit (%d bytes)", cfg.MaxTotalBytes)
					}

					newParts = append(newParts, map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url":    rendered.DataURL,
							"detail": "high",
						},
					})
				}
			}

		case []any:
			for _, p := range cv {
				pMap, ok := p.(map[string]any)
				if !ok {
					newParts = append(newParts, p)
					continue
				}

				pType, _ := pMap["type"].(string)
				switch strings.ToLower(pType) {
				case "image_url":
					// Native image: 100% PRESERVE intact without splitting or modifying
					newParts = append(newParts, pMap)

				case "text":
					txt, _ := pMap["text"].(string)
					if strings.TrimSpace(txt) == "" {
						newParts = append(newParts, pMap)
						continue
					}
					chunks := chunker.ChunkText(txt)
					for _, chk := range chunks {
						totalImagesCount++
						if cfg.MaxPages > 0 && totalImagesCount > cfg.MaxPages {
							if cfg.FallbackOnError {
								return bodyBytes, true, nil
							}
							return bodyBytes, false, fmt.Errorf("request exceeded max pages limit (%d)", cfg.MaxPages)
						}

						rendered, err := rasterizer.RenderChunk(roleLower, chk)
						if err != nil {
							if cfg.FallbackOnError {
								return bodyBytes, true, nil
							}
							return bodyBytes, false, fmt.Errorf("rasterization error on msg %d: %w", msgIdx, err)
						}

						totalImagesBytes += rendered.ByteLength
						if cfg.MaxTotalBytes > 0 && totalImagesBytes > cfg.MaxTotalBytes {
							if cfg.FallbackOnError {
								return bodyBytes, true, nil
							}
							return bodyBytes, false, fmt.Errorf("request exceeded total image bytes limit (%d bytes)", cfg.MaxTotalBytes)
						}

						newParts = append(newParts, map[string]any{
							"type": "image_url",
							"image_url": map[string]any{
								"url":    rendered.DataURL,
								"detail": "high",
							},
						})
					}

				default:
					newParts = append(newParts, pMap)
				}
			}

		default:
			newParts = append(newParts, map[string]any{
				"type": "text",
				"text": fmt.Sprintf("%v", rawContent),
			})
		}

		clonedMsg := make(map[string]any)
		for k, v := range msgMap {
			clonedMsg[k] = v
		}

		if len(newParts) > 0 {
			clonedMsg["content"] = newParts
		}
		newMessages = append(newMessages, clonedMsg)
	}

	rootMap["messages"] = newMessages
	transformedBytes, err := json.Marshal(rootMap)
	if err != nil {
		if cfg.FallbackOnError {
			return bodyBytes, true, nil
		}
		return bodyBytes, false, fmt.Errorf("failed to marshal transformed request: %w", err)
	}

	return transformedBytes, false, nil
}
