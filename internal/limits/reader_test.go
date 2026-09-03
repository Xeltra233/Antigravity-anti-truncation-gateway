package limits

import (
	"errors"
	"strings"
	"testing"
)

func TestReadAllWithinLimit(t *testing.T) {
	data, err := ReadAll(strings.NewReader("12345"), 5)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(data) != "12345" {
		t.Fatalf("data = %q, want %q", data, "12345")
	}
}

func TestReadAllExceedsLimit(t *testing.T) {
	data, err := ReadAll(strings.NewReader("123456"), 5)
	if !errors.Is(err, ErrExceeded) {
		t.Fatalf("error = %v, want ErrExceeded", err)
	}
	if data != nil {
		t.Fatalf("data = %q, want nil on limit error", data)
	}
}

func TestReadAllUnlimited(t *testing.T) {
	data, err := ReadAll(strings.NewReader("unlimited"), 0)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(data) != "unlimited" {
		t.Fatalf("data = %q, want %q", data, "unlimited")
	}
}
