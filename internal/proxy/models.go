package proxy

import (
	"encoding/json"
	"fmt"
	"strings"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/imagectx"
	"antigravity-gateway/internal/synthetic"
)

// FormatTriModelList inspects upstream models and duplicates eligible text models into 3 variants:
// 1. [抗截断] <model_id> (Standard text anti-truncation mode)
// 2. [实验性] <model_id> (Full image stream mode)
// 3. [混合实验性] <model_id> (Hybrid latest-turn-only image stream mode)
// While strictly preserving image generation and non-text models as single entries without prefix.
func FormatTriModelList(rawBytes []byte, cfg *config.Config, filter *synthetic.ModelFilter) ([]byte, error) {
	if cfg == nil {
		return rawBytes, nil
	}

	stdPrefix := cfg.StandardAliasPrefix
	if stdPrefix == "" {
		stdPrefix = "[抗截断] "
	}
	expPrefix := cfg.ExperimentalAliasPrefix
	if expPrefix == "" {
		expPrefix = "[实验性] "
	}
	hybPrefix := cfg.HybridAliasPrefix
	if hybPrefix == "" {
		hybPrefix = "[混合实验性] "
	}

	var rootMap map[string]any
	if err := json.Unmarshal(rawBytes, &rootMap); err != nil {
		return rawBytes, fmt.Errorf("failed to parse upstream models json: %w", err)
	}

	rawList, ok := rootMap["data"].([]any)
	if !ok || len(rawList) == 0 {
		return rawBytes, nil
	}

	if filter == nil {
		filter = synthetic.NewModelFilter(cfg.TextModelPattern, cfg.NonTextModelPattern)
	}

	var newList []any
	seenIDs := make(map[string]bool)

	for _, item := range rawList {
		mMap, ok := item.(map[string]any)
		if !ok {
			newList = append(newList, item)
			continue
		}

		modelID, _ := mMap["id"].(string)
		trimmedID := strings.TrimSpace(modelID)
		if trimmedID == "" {
			newList = append(newList, item)
			continue
		}

		// 1. If Image Generation or Non-Text: Output 100% single raw entry without prefix
		if imagectx.IsImageGenerationRequest(trimmedID, "") || !filter.IsTextModel(trimmedID) {
			if !seenIDs[trimmedID] {
				seenIDs[trimmedID] = true
				newList = append(newList, mMap)
			}
			continue
		}

		// 2. For Text Models: Output 3 distinct variants
		prefixes := []string{stdPrefix, expPrefix, hybPrefix}
		for _, pfx := range prefixes {
			variantID := pfx + trimmedID
			if !seenIDs[variantID] {
				seenIDs[variantID] = true
				variantMap := make(map[string]any)
				for k, v := range mMap {
					variantMap[k] = v
				}
				variantMap["id"] = variantID
				newList = append(newList, variantMap)
			}
		}
	}

	rootMap["data"] = newList

	formattedJSON, err := json.Marshal(rootMap)
	if err != nil {
		return rawBytes, fmt.Errorf("failed to marshal formatted models json: %w", err)
	}

	return formattedJSON, nil
}
