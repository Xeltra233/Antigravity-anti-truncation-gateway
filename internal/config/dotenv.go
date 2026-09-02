package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// LoadDotEnv searches for and loads a .env file into the process environment.
// It searches in the following order:
//  1. Custom paths explicitly passed to this function (if any)
//  2. Custom path from the ENV_FILE environment variable (if set)
//  3. Current working directory (.env)
//  4. Executable directory (filepath.Dir(os.Executable()) + "/.env")
//  5. Parent directory (../.env, convenient for tests / dev)
//
// Existing non-empty environment variables in the OS take precedence over .env values (12-Factor App rule).
// Returns the path of the loaded file (empty if no file found) and the number of variables injected.
func LoadDotEnv(customPaths ...string) (string, int, error) {
	candidates := make([]string, 0, 8)

	// 1. Explicit paths passed to function
	for _, p := range customPaths {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			candidates = append(candidates, trimmed)
		}
	}

	// 2. Custom path from ENV_FILE environment variable
	if envPath := strings.TrimSpace(os.Getenv("ENV_FILE")); envPath != "" {
		candidates = append(candidates, envPath)
	}

	// 3. Current working directory
	candidates = append(candidates, ".env")

	// 4. Executable directory (critical for Windows double-click and background services)
	if exe, err := os.Executable(); err == nil && exe != "" {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, ".env"))
	}

	// 5. Parent directory (for dev / tests run from subdirectories)
	candidates = append(candidates, "../.env")

	var targetFile string
	seen := make(map[string]bool)
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			abs = c
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true

		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			targetFile = c
			break
		}
	}

	if targetFile == "" {
		return "", 0, nil // No .env file found; valid when running with pure system env vars
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		return targetFile, 0, fmt.Errorf("failed to read .env file (%s): %w", targetFile, err)
	}

	envMap, err := ParseDotEnv(string(data))
	if err != nil {
		return targetFile, 0, fmt.Errorf("failed to parse .env file (%s): %w", targetFile, err)
	}

	loadedCount := 0
	for k, v := range envMap {
		// Existing non-empty environment variables always take precedence
		if existing, exists := os.LookupEnv(k); !exists || strings.TrimSpace(existing) == "" {
			if err := os.Setenv(k, v); err == nil {
				loadedCount++
			}
		}
	}

	return targetFile, loadedCount, nil
}

// ParseDotEnv parses standard .env file contents into key-value pairs.
// Handles UTF-8 BOM, comments (#), export prefixes, single/double quotes, escape sequences, and multiline values.
func ParseDotEnv(content string) (map[string]string, error) {
	result := make(map[string]string)

	// Strip UTF-8 BOM if present (frequent when editing .env with Windows Notepad)
	content = strings.TrimPrefix(content, "\ufeff")

	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentKey string
	var currentValue strings.Builder
	var inQuote byte // '"' or '\'' or 0
	var isMultiline bool

	for scanner.Scan() {
		line := scanner.Text()

		if isMultiline {
			currentValue.WriteString("\n")
			closed := false
			for i := 0; i < len(line); i++ {
				if line[i] == inQuote {
					// Check for escape if double quote
					if inQuote == '"' && i > 0 && line[i-1] == '\\' {
						currentValue.WriteByte(line[i])
						continue
					}
					// Found closing quote
					currentValue.WriteString(line[:i])
					closed = true
					break
				}
			}
			if closed {
				val := currentValue.String()
				if inQuote == '"' {
					val = unescapeDoubleQuoted(val)
				}
				result[currentKey] = val
				currentKey = ""
				currentValue.Reset()
				inQuote = 0
				isMultiline = false
			} else {
				currentValue.WriteString(line)
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Support "export KEY=VAL"
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}

		idx := strings.Index(trimmed, "=")
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])

		if key == "" {
			continue
		}

		// Check if quoted
		if len(val) > 0 && (val[0] == '"' || val[0] == '\'') {
			quote := val[0]
			endIdx := -1
			for i := 1; i < len(val); i++ {
				if val[i] == quote {
					if quote == '"' && val[i-1] == '\\' {
						continue
					}
					endIdx = i
					break
				}
			}

			if endIdx >= 0 {
				// Closed on same line
				inner := val[1:endIdx]
				if quote == '"' {
					inner = unescapeDoubleQuoted(inner)
				}
				result[key] = inner
			} else {
				// Multiline quote started
				currentKey = key
				inQuote = quote
				isMultiline = true
				currentValue.WriteString(val[1:])
			}
		} else {
			// Unquoted: strip inline comments if any (" # comment")
			val = stripInlineComment(val)
			result[key] = val
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// If file ended with unclosed multiline quote, store whatever was collected
	if isMultiline && currentKey != "" {
		val := currentValue.String()
		if inQuote == '"' {
			val = unescapeDoubleQuoted(val)
		}
		result[currentKey] = val
	}

	return result, nil
}

func stripInlineComment(val string) string {
	var inEscape bool
	for i := 0; i < len(val); i++ {
		if inEscape {
			inEscape = false
			continue
		}
		if val[i] == '\\' {
			inEscape = true
			continue
		}
		if val[i] == '#' {
			// Only treat as comment if preceded by whitespace
			if i > 0 && unicode.IsSpace(rune(val[i-1])) {
				return strings.TrimSpace(val[:i-1])
			}
		}
	}
	return strings.TrimSpace(val)
}

func unescapeDoubleQuoted(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'n':
				b.WriteByte('\n')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case '"':
				b.WriteByte('"')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
