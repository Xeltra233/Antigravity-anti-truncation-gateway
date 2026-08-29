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
				// Might start directly with key
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
				// Safety check: key too long or unexpected structure
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
				// Non-whitespace character before quote
				d.state = StateError
			}
			i++

		case StateInString:
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
					// End of content string!
					d.state = StateCompleted
					wIdx++
					i = len(src)
					break
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
	if d.state == StateCompleted {
		return "", nil
	}

	// If never reached completion (e.g. malformed JSON or unclosed quote), attempt bounded repair
	raw := d.accumRaw.String()
	if raw == "" {
		return "", nil
	}

	res, err := RepairSyntheticArguments(raw, maxBytes)
	if err != nil {
		return "", err
	}

	// If we haven't emitted anything yet, return the whole repaired content
	if !d.ContentEmitted {
		d.ContentEmitted = true
		return res.Content, nil
	}

	return "", nil
}

func utf16IsHighSurrogate(r rune) bool {
	return 0xD800 <= r && r <= 0xDBFF
}

func utf16IsLowSurrogate(r rune) bool {
	return 0xDC00 <= r && r <= 0xDFFF
}

func utf16DecodeSurrogate(high, low rune) rune {
	return 0x10000 + (high-0xD800)<<10 + (low - 0xDC00)
}
