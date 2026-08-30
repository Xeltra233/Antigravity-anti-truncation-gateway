package synthetic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrArgumentsDecodeFailed  = errors.New("synthetic tool arguments decode failed")
	ErrInvalidArgumentsSchema = errors.New("synthetic tool arguments must be an object with string 'content'")
)

type RepairResult struct {
	Content  string
	Repaired bool
}

// RepairSyntheticArguments attempts to parse or repair the JSON arguments of a synthetic tool call.
func RepairSyntheticArguments(rawArgs string, maxBytes int) (*RepairResult, error) {
	if len(rawArgs) > maxBytes {
		return nil, fmt.Errorf("arguments exceed maximum allowed size (%d bytes)", maxBytes)
	}

	trimmed := strings.TrimSpace(rawArgs)
	if trimmed == "" {
		return nil, ErrArgumentsDecodeFailed
	}

	// 1. Fast path: standard json parse
	var target struct {
		Content *string `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &target); err == nil && target.Content != nil {
		return &RepairResult{
			Content:  *target.Content,
			Repaired: false,
		}, nil
	}

	// 2. Strip Markdown code fences: e.g. ```json ... ```
	cleaned := stripMarkdownCodeFences(trimmed)

	target.Content = nil
	if err := json.Unmarshal([]byte(cleaned), &target); err == nil && target.Content != nil {
		return &RepairResult{
			Content:  *target.Content,
			Repaired: true,
		}, nil
	}

	// 3. Structural repair: fix unclosed strings, trailing commas, missing closing braces
	repairedJSON := attemptStructuralRepair(cleaned)
	target.Content = nil
	if err := json.Unmarshal([]byte(repairedJSON), &target); err == nil && target.Content != nil {
		return &RepairResult{
			Content:  *target.Content,
			Repaired: true,
		}, nil
	}

	// 4. Bounded scanner fallback for `content` field
	contentStr, ok := scanAndExtractContent(cleaned)
	if ok {
		return &RepairResult{
			Content:  contentStr,
			Repaired: true,
		}, nil
	}

	return nil, ErrArgumentsDecodeFailed
}

func stripMarkdownCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence line
		firstNewline := strings.IndexByte(s, '\n')
		if firstNewline != -1 {
			s = s[firstNewline+1:]
		} else {
			s = strings.TrimPrefix(s, "```")
		}
		// Remove closing fence
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

func attemptStructuralRepair(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		idx := strings.IndexByte(s, '{')
		if idx != -1 {
			s = s[idx:]
		} else {
			return s
		}
	}

	// Track in-string state, open braces and brackets
	var buf bytes.Buffer
	inString := false
	escaped := false
	openBraces := 0
	openBrackets := 0

	for i := 0; i < len(s); i++ {
		b := s[i]
		if inString {
			if escaped {
				escaped = false
				buf.WriteByte(b)
			} else if b == '\\' {
				escaped = true
				buf.WriteByte(b)
			} else if b == '"' {
				inString = false
				buf.WriteByte(b)
			} else if b == '\n' || b == '\r' || b == '\t' {
				// Escape raw unescaped control chars in JSON string
				switch b {
				case '\n':
					buf.WriteString(`\n`)
				case '\r':
					buf.WriteString(`\r`)
				case '\t':
					buf.WriteString(`\t`)
				}
			} else {
				buf.WriteByte(b)
			}
		} else {
			if b == '"' {
				inString = true
				buf.WriteByte(b)
			} else if b == '{' {
				openBraces++
				buf.WriteByte(b)
			} else if b == '}' {
				if openBraces > 0 {
					openBraces--
					buf.WriteByte(b)
				}
			} else if b == '[' {
				openBrackets++
				buf.WriteByte(b)
			} else if b == ']' {
				if openBrackets > 0 {
					openBrackets--
					buf.WriteByte(b)
				}
			} else {
				buf.WriteByte(b)
			}
		}
	}

	// If terminated inside string, close quote
	if inString {
		buf.WriteByte('"')
	}

	// Remove trailing commas before closing
	res := strings.TrimRight(buf.String(), " \t\r\n,")

	// Balance brackets and braces
	for openBrackets > 0 {
		res += "]"
		openBrackets--
	}
	for openBraces > 0 {
		res += "}"
		openBraces--
	}

	return res
}

// scanAndExtractContent scans for `"content"\s*:\s*"` and unescapes the value.
func scanAndExtractContent(s string) (string, bool) {
	keyIdx := strings.Index(s, `"content"`)
	if keyIdx == -1 {
		return "", false
	}
	sub := s[keyIdx+len(`"content"`):]
	colonIdx := strings.IndexByte(sub, ':')
	if colonIdx == -1 {
		return "", false
	}
	sub = strings.TrimSpace(sub[colonIdx+1:])
	if !strings.HasPrefix(sub, `"`) {
		return "", false
	}

	// Scan string literal
	var valBuf bytes.Buffer
	escaped := false
	started := false

	for i := 0; i < len(sub); i++ {
		b := sub[i]
		if !started {
			if b == '"' {
				started = true
			}
			continue
		}

		if escaped {
			escaped = false
			switch b {
			case '"', '\\', '/':
				valBuf.WriteByte(b)
			case 'b':
				valBuf.WriteByte('\b')
			case 'f':
				valBuf.WriteByte('\f')
			case 'n':
				valBuf.WriteByte('\n')
			case 'r':
				valBuf.WriteByte('\r')
			case 't':
				valBuf.WriteByte('\t')
			case 'u':
				if i+4 < len(sub) {
					hexStr := sub[i+1 : i+5]
					if u, err := strconv.ParseUint(hexStr, 16, 16); err == nil {
						var rBuf [4]byte
						n := utf8.EncodeRune(rBuf[:], rune(u))
						valBuf.Write(rBuf[:n])
						i += 4
						continue
					}
				}
				valBuf.WriteString(`\u`)
			default:
				valBuf.WriteByte(b)
			}
		} else if b == '\\' {
			escaped = true
		} else if b == '"' {
			// Check if this quote is the true closing quote (followed only by whitespace and '}' or ',')
			rest := strings.TrimLeft(sub[i+1:], " \t\r\n")
			if rest == "" || strings.HasPrefix(rest, "}") || strings.HasPrefix(rest, ",") {
				// Found true closing quote
				return valBuf.String(), true
			} else {
				// Internal literal quote
				valBuf.WriteByte('"')
			}
		} else {
			valBuf.WriteByte(b)
		}
	}

	// Even if unclosed at the very end, if we collected content, return it
	if valBuf.Len() > 0 {
		return valBuf.String(), true
	}

	return "", false
}
