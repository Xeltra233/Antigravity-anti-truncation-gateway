package synthetic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"antigravity-gateway/internal/config"
)

type InjectedRequest struct {
	SyntheticToolName  string
	TransformedBody    []byte
	OriginalModel      string
	IsStreaming        bool
	HasDownstreamTools bool
	ForcedRealTool     bool
}

type RequestInjector struct {
	cfg         *config.Config
	modelFilter *ModelFilter
}

func NewRequestInjector(cfg *config.Config) *RequestInjector {
	return &RequestInjector{
		cfg:         cfg,
		modelFilter: NewModelFilter(cfg.TextModelPattern, cfg.NonTextModelPattern),
	}
}

func (inj *RequestInjector) GenerateUniqueToolName(existingToolNames map[string]bool) (string, error) {
	prefix := inj.cfg.SyntheticToolPrefix
	if prefix == "" {
		prefix = "agw_emit_"
	}
	for i := 0; i < 100; i++ {
		// 96 bits = 12 bytes = 24 hex characters
		nonceBytes := make([]byte, 12)
		if _, err := rand.Read(nonceBytes); err != nil {
			return "", fmt.Errorf("failed to generate random nonce: %w", err)
		}
		nonce := hex.EncodeToString(nonceBytes)
		toolName := prefix + nonce
		if !existingToolNames[toolName] {
			return toolName, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique synthetic tool name after 100 attempts")
}

func (inj *RequestInjector) BuildSyntheticTool(toolName string) map[string]any {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"content"},
	}

	if inj.cfg.SyntheticToolStrict {
		params["additionalProperties"] = false
	}

	fn := map[string]any{
		"name":        toolName,
		"description": "Use this transport tool exactly once for the final user-visible answer. Put the complete answer in content. Never wrap genuine tool calls in it.",
		"parameters":  params,
	}

	if inj.cfg.SyntheticToolStrict {
		fn["strict"] = true
	}

	return map[string]any{
		"type":     "function",
		"function": fn,
	}
}

func (inj *RequestInjector) BuildControlMessage(toolName string) map[string]any {
	role := inj.cfg.ControlMessageRole
	if role == "" {
		role = "user"
	}

	prompt := inj.cfg.ControlPromptTemplate
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Always use tool `%s` to output your final reply in its `content` argument. Do not output anything outside this tool call.",
			toolName,
		)
	} else {
		prompt = fmt.Sprintf(prompt, toolName)
	}

	return map[string]any{
		"role":    role,
		"content": prompt,
	}
}

func (inj *RequestInjector) Inject(rawBody []byte) (*InjectedRequest, error) {
	return inj.InjectContext(context.Background(), rawBody)
}

func (inj *RequestInjector) InjectContext(ctx context.Context, rawBody []byte) (*InjectedRequest, error) {
	var bodyMap map[string]any
	if err := json.Unmarshal(rawBody, &bodyMap); err != nil {
		return nil, fmt.Errorf("invalid json in request body: %w", err)
	}

	modelStr, _ := bodyMap["model"].(string)
	isStreaming, _ := bodyMap["stream"].(bool)

	// If WRAPPER_MODE is "off" or model is NOT a text model (e.g. image, audio, embedding), pass through verbatim
	if inj.cfg.WrapperMode == "off" || !inj.modelFilter.IsTextModel(modelStr) {
		return &InjectedRequest{
			SyntheticToolName:  "",
			TransformedBody:    rawBody,
			OriginalModel:      modelStr,
			IsStreaming:        isStreaming,
			HasDownstreamTools: false,
			ForcedRealTool:     false,
		}, nil
	}

	// Collect existing tool names
	existingToolNames := make(map[string]bool)
	var rawTools []any
	hasDownstreamTools := false
	if tList, ok := bodyMap["tools"].([]any); ok && len(tList) > 0 {
		hasDownstreamTools = true
		rawTools = append(rawTools, tList...)
		for _, t := range tList {
			if tMap, ok := t.(map[string]any); ok {
				if fnMap, ok := tMap["function"].(map[string]any); ok {
					if name, ok := fnMap["name"].(string); ok {
						existingToolNames[name] = true
					}
				}
			}
		}
	}

	// Generate unique synthetic tool name
	toolName, err := inj.GenerateUniqueToolName(existingToolNames)
	if err != nil {
		return nil, err
	}

	// Append synthetic tool to tools
	syntheticTool := inj.BuildSyntheticTool(toolName)
	tools := append(rawTools, syntheticTool)
	bodyMap["tools"] = tools

	// Handle tool_choice mapping
	forcedRealTool := false
	if tc, ok := bodyMap["tool_choice"]; ok {
		switch v := tc.(type) {
		case string:
			if v == "none" {
				bodyMap["tool_choice"] = "auto"
			} else if v == "required" {
				bodyMap["tool_choice"] = "required"
			} else if v == "auto" {
				bodyMap["tool_choice"] = "auto"
			}
		case map[string]any:
			forcedRealTool = true
			bodyMap["tool_choice"] = v
		}
	} else {
		bodyMap["tool_choice"] = "auto"
	}

	// Prepare messages
	var messages []any
	if mList, ok := bodyMap["messages"].([]any); ok {
		messages = append(messages, mList...)
	}

	controlMsg := inj.BuildControlMessage(toolName)

	if len(messages) > 0 {
		lastMsg, _ := messages[len(messages)-1].(map[string]any)
		lastRole, _ := lastMsg["role"].(string)

		// Gemini / CPA 上游铁律：请求绝不能以 assistant/model 轮次结束，否则报错 400 "Requests ending with a model turn are not supported."
		if lastRole == "assistant" {
			userControl := map[string]any{
				"role":    "user",
				"content": controlMsg["content"],
			}
			messages = append(messages, userControl)
		} else {
			switch inj.cfg.ControlMessagePosition {
			case "head":
				messages = append([]any{controlMsg}, messages...)
			case "system_tail":
				lastSystemIdx := -1
				for i, m := range messages {
					if mMap, ok := m.(map[string]any); ok {
						role, _ := mMap["role"].(string)
						if role == "system" || role == "developer" {
							lastSystemIdx = i
						}
					}
				}
				if lastSystemIdx >= 0 {
					newMessages := make([]any, 0, len(messages)+1)
					newMessages = append(newMessages, messages[:lastSystemIdx+1]...)
					newMessages = append(newMessages, controlMsg)
					newMessages = append(newMessages, messages[lastSystemIdx+1:]...)
					messages = newMessages
				} else {
					messages = append([]any{controlMsg}, messages...)
				}
			default: // "tail"
				messages = append(messages, controlMsg)
			}
		}
	} else {
		messages = append(messages, controlMsg)
	}

	bodyMap["messages"] = messages

	transformed, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transformed request: %w", err)
	}

	return &InjectedRequest{
		SyntheticToolName:  toolName,
		TransformedBody:    transformed,
		OriginalModel:      modelStr,
		IsStreaming:        isStreaming,
		HasDownstreamTools: hasDownstreamTools,
		ForcedRealTool:     forcedRealTool,
	}, nil
}
