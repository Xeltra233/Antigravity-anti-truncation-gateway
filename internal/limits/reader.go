package limits

import (
	"errors"
	"io"
)

var ErrExceeded = errors.New("configured byte limit exceeded")

// Reader enforces a hard byte limit without returning the first byte beyond the
// limit to callers. Once the underlying stream contains more data, Read returns
// ErrExceeded.
type Reader struct {
	source    io.Reader
	remaining int64
	exceeded  bool
	unlimited bool
}

func NewReader(source io.Reader, maxBytes int64) *Reader {
	return &Reader{
		source:    source,
		remaining: maxBytes,
		unlimited: maxBytes <= 0,
	}
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.unlimited {
		return r.source.Read(p)
	}
	if r.exceeded {
		return 0, ErrExceeded
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining > 0 {
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
		n, err := r.source.Read(p)
		r.remaining -= int64(n)
		return n, err
	}

	var probe [1]byte
	n, err := r.source.Read(probe[:])
	if n > 0 {
		r.exceeded = true
		return 0, ErrExceeded
	}
	return 0, err
}

func ReadAll(source io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(NewReader(source, maxBytes))
	if err != nil {
		return nil, err
	}
	return data, nil
}
