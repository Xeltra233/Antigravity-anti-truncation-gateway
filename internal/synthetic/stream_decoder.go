package synthetic

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

type StreamDecoderState int

const (
	StateInit StreamDecoderState = iota
	StateLookingForKey
	StateLookingForColon
	StateLookingForQuote
	StateInString
	StateCompleted
	StateError
)

type IncrementalJSONStringDecoder struct {
	state          StreamDecoderState
	pendingEscape  []byte
	pendingUTF8    []byte
	pendingKey     strings.Builder
	pendingQuote   bool // A trailing quote held at the end of a chunk to see if next chunk is '}'
	surrogateHigh  rune
	accumRaw       strings.Builder
	ContentEmitted bool
}

func NewIncrementalJSONStringDecoder() *IncrementalJSONStringDecoder {
	return &IncrementalJSONStringDecoder{
		state: StateInit,
	}
}

func (d *IncrementalJSONStringDecoder) Feed(chunk string) (string, bool, error) {
	if chunk == "" {
		return "", d.state == StateCompleted, nil
	}

	d.accumRaw.WriteString(chunk)

	var out bytes.Buffer
	src := []byte(chunk)
	i := 0

	for i < len(src) {
		switch d.state {
		case StateInit:
			// Look for { or "content"
			if src[i] == '{' {
				d.state = StateLookingForKey
			} else if src[i] == '"' {
				d.state = StateLookingForKey
				d.pendingKey.WriteByte('"')
			}
			i++

		case StateLookingForKey:
			d.pendingKey.WriteByte(src[i])
			i++
			keyStr := d.pendingKey.String()
			if strings.Contains(keyStr, `"content"`) {
				d.state = StateLookingForColon
				d.pendingKey.Reset()
			} else if len(keyStr) > 500 {
				d.state = StateError
			}

		case StateLookingForColon:
			if src[i] == ':' {
				d.state = StateLookingForQuote
			}
			i++

		case StateLookingForQuote:
			if src[i] == '"' {
				d.state = StateInString
			} else if src[i] != ' ' && src[i] != '\t' && src[i] != '\r' && src[i] != '\n' {
				d.state = StateError
			}
			i++

		case StateInString:
			// If we held a pending quote from the previous chunk, determine if it was closing or literal
			if d.pendingQuote {
				d.pendingQuote = false
				// Check if current chunk starts with optional whitespace + '}'
				trimmedRemaining := strings.TrimLeft(string(src[i:]), " \t\r\n")
				if strings.HasPrefix(trimmedRemaining, "}") || trimmedRemaining == "" {
					// It is the closing quote followed by '}'
					d.state = StateCompleted
					i = len(src)
					break
				} else {
					// It was a literal quote!
					out.WriteByte('"')
				}
			}

			// Combine any pending prefix
			var workingBytes []byte
			if len(d.pendingEscape) > 0 {
				workingBytes = append(d.pendingEscape, src[i:]...)
				d.pendingEscape = nil
			} else if len(d.pendingUTF8) > 0 {
				workingBytes = append(d.pendingUTF8, src[i:]...)
				d.pendingUTF8 = nil
			} else {
				workingBytes = src[i:]
			}

			wIdx := 0
			for wIdx < len(workingBytes) {
				b := workingBytes[wIdx]

				if b == '\\' {
					// Escape sequence started
					if wIdx+1 >= len(workingBytes) {
						// Trailing backslash, hold for next chunk
						d.pendingEscape = workingBytes[wIdx:]
						i = len(src)
						break
					}
					next := workingBytes[wIdx+1]
					switch next {
					case '"':
						out.WriteByte('"')
						wIdx += 2
					case '\\':
						out.WriteByte('\\')
						wIdx += 2
					case '/':
						out.WriteByte('/')
						wIdx += 2
					case 'b':
						out.WriteByte('\b')
						wIdx += 2
					case 'f':
						out.WriteByte('\f')
						wIdx += 2
					case 'n':
						out.WriteByte('\n')
						wIdx += 2
					case 'r':
						out.WriteByte('\r')
						wIdx += 2
					case 't':
						out.WriteByte('\t')
						wIdx += 2
					case 'u':
						// Unicode \uXXXX
						if wIdx+6 > len(workingBytes) {
							// Incomplete unicode escape, hold
							d.pendingEscape = workingBytes[wIdx:]
							wIdx = len(workingBytes)
							break
						}
						hexStr := string(workingBytes[wIdx+2 : wIdx+6])
						uVal, err := strconv.ParseUint(hexStr, 16, 16)
						if err != nil {
							out.WriteString(`\u` + hexStr)
						} else {
							r := rune(uVal)
							if utf16IsHighSurrogate(r) {
								d.surrogateHigh = r
							} else if utf16IsLowSurrogate(r) && d.surrogateHigh != 0 {
								combined := utf16DecodeSurrogate(d.surrogateHigh, r)
								var rBuf [4]byte
								n := utf8.EncodeRune(rBuf[:], combined)
								out.Write(rBuf[:n])
								d.surrogateHigh = 0
							} else {
								var rBuf [4]byte
								n := utf8.EncodeRune(rBuf[:], r)
								out.Write(rBuf[:n])
							}
						}
						wIdx += 6
					default:
						// Unknown escape, output verbatim
						out.WriteByte(b)
						out.WriteByte(next)
						wIdx += 2
					}
				} else if b == '"' {
					// We encountered a quote '"'. Check if it's the closing quote or an unescaped literal quote.
					remainingAfterQuote := workingBytes[wIdx+1:]
					trimmedRest := strings.TrimLeft(string(remainingAfterQuote), " \t\r\n")

					if len(remainingAfterQuote) == 0 {
						// Quote is at the very end of this chunk. Hold it to inspect the start of the next chunk.
						d.pendingQuote = true
						wIdx++
						i = len(src)
						break
					} else if strings.HasPrefix(trimmedRest, "}") {
						// Followed immediately by '}' -> definitely the closing quote!
						d.state = StateCompleted
						wIdx = len(workingBytes)
						i = len(src)
						break
					} else {
						// Followed by regular characters -> unescaped literal quote in text!
						out.WriteByte('"')
						wIdx++
					}
				} else {
					// UTF-8 rune handling
					if b < 0x80 {
						out.WriteByte(b)
						wIdx++
					} else {
						// Multi-byte sequence
						if !utf8.FullRune(workingBytes[wIdx:]) {
							// Incomplete UTF-8 sequence at end of buffer
							d.pendingUTF8 = workingBytes[wIdx:]
							wIdx = len(workingBytes)
							break
						}
						r, size := utf8.DecodeRune(workingBytes[wIdx:])
						if r == utf8.RuneError && size == 1 {
							out.WriteByte(b)
							wIdx++
						} else {
							var rBuf [4]byte
							n := utf8.EncodeRune(rBuf[:], r)
							out.Write(rBuf[:n])
							wIdx += size
						}
					}
				}
			}
			i = len(src)

		case StateCompleted:
			// If we are marked completed but more non-closing text comes in (e.g. false closing quote), resume streaming
			trimmed := strings.TrimSpace(string(src[i:]))
			if trimmed != "" && trimmed != "}" && trimmed != "}," && trimmed != "}\n" {
				d.state = StateInString
				continue
			}
			i = len(src)

		case StateError:
			i = len(src)
		}
	}

	result := out.String()
	if result != "" {
		d.ContentEmitted = true
	}
	return result, d.state == StateCompleted, nil
}

func (d *IncrementalJSONStringDecoder) Finish(maxBytes int) (string, error) {
	var out bytes.Buffer

	if d.pendingQuote && !d.ContentEmitted {
		out.WriteByte('"')
		d.pendingQuote = false
	}

	if len(d.pendingEscape) > 0 {
		out.Write(d.pendingEscape)
		d.pendingEscape = nil
	}
	if len(d.pendingUTF8) > 0 {
		out.Write(d.pendingUTF8)
		d.pendingUTF8 = nil
	}

	// If no content was emitted throughout the entire stream, use the robust RepairSyntheticArguments fallback
	if !d.ContentEmitted {
		raw := d.accumRaw.String()
		if raw != "" {
			res, err := RepairSyntheticArguments(raw, maxBytes)
			if err == nil && res.Content != "" {
				return res.Content, nil
			}
		}
	}

	return out.String(), nil
}

func utf16IsHighSurrogate(r rune) bool {
	return r >= 0xD800 && r <= 0xDBFF
}

func utf16IsLowSurrogate(r rune) bool {
	return r >= 0xDC00 && r <= 0xDFFF
}

func utf16DecodeSurrogate(high, low rune) rune {
	return 0x10000 + ((high - 0xD800) << 10) + (low - 0xDC00)
}
