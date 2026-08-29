package synthetic

import (
	"testing"
)

func TestRepairSyntheticArguments(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantContent string
		wantRepair  bool
		wantErr     bool
	}{
		{
			name:        "valid standard JSON",
			input:       `{"content": "hello world"}`,
			wantContent: "hello world",
			wantRepair:  false,
			wantErr:     false,
		},
		{
			name:        "markdown code fence",
			input:       "```json\n{\"content\": \"fenced content\"}\n```",
			wantContent: "fenced content",
			wantRepair:  true,
			wantErr:     false,
		},
		{
			name:        "unclosed quote and brace",
			input:       `{"content": "unclosed text`,
			wantContent: "unclosed text",
			wantRepair:  true,
			wantErr:     false,
		},
		{
			name:        "trailing comma",
			input:       `{"content": "trailing",}`,
			wantContent: "trailing",
			wantRepair:  true,
			wantErr:     false,
		},
		{
			name:        "raw unescaped newlines in string",
			input:       "{\"content\": \"line1\nline2\nline3\"}",
			wantContent: "line1\nline2\nline3",
			wantRepair:  true,
			wantErr:     false,
		},
		{
			name:        "unicode escape in string",
			input:       `{"content": "Hello \u4e16\u754c"}`,
			wantContent: "Hello 世界",
			wantRepair:  false,
			wantErr:     false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid non-content object",
			input:   `{"something_else": 123}`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := RepairSyntheticArguments(tc.input, 1024*1024)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (content: %q)", res.Content)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Content != tc.wantContent {
				t.Errorf("content mismatch: got %q, want %q", res.Content, tc.wantContent)
			}
			if res.Repaired != tc.wantRepair {
				t.Errorf("repair flag mismatch: got %v, want %v", res.Repaired, tc.wantRepair)
			}
		})
	}
}
