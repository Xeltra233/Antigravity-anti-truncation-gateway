package config

import (
	"testing"
	"time"
)

func TestConfigLoadSuccess(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_BASE_URL": "https://api.example.com",
		"UPSTREAM_API_KEY":  "sk-upstream-key",
		"ADMIN_API_KEY":     "admin-secret-key-12345",
		"KEY_HMAC_SECRET":   "very-secure-hmac-secret-12345",
		"DOWNSTREAM_KEYS_JSON": `[
			{"id": "k1", "key": "sk-1", "name": "test key 1", "allowed_models": ["gpt-4o"]}
		]`,
	}

	cfg, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.UpstreamBaseURL != "https://api.example.com" {
		t.Errorf("expected https://api.example.com, got %s", cfg.UpstreamBaseURL)
	}
	if cfg.UpstreamChatURL() != "https://api.example.com/v1/chat/completions" {
		t.Errorf("expected https://api.example.com/v1/chat/completions, got %s", cfg.UpstreamChatURL())
	}
	if cfg.UpstreamModelsURL() != "https://api.example.com/v1/models" {
		t.Errorf("expected https://api.example.com/v1/models, got %s", cfg.UpstreamModelsURL())
	}
	if cfg.UpstreamAuthMode != "bearer" {
		t.Errorf("expected bearer, got %s", cfg.UpstreamAuthMode)
	}
	if len(cfg.StaticKeys) != 1 || cfg.StaticKeys[0].ID != "k1" {
		t.Errorf("expected 1 static key, got %v", cfg.StaticKeys)
	}
	if cfg.WrapperMode != "prefer" {
		t.Errorf("expected prefer, got %s", cfg.WrapperMode)
	}
	if cfg.ControlMessageRole != "system" {
		t.Errorf("expected system, got %s", cfg.ControlMessageRole)
	}
	if cfg.UpstreamTimeout != 120*time.Second {
		t.Errorf("expected 120s, got %v", cfg.UpstreamTimeout)
	}
}

func TestConfigRootURLNormalization(t *testing.T) {
	variations := []string{
		"https://example.com",
		"https://example.com/",
		"https://example.com/v1",
		"https://example.com/v1/",
	}
	for _, v := range variations {
		env := map[string]string{
			"UPSTREAM_BASE_URL": v,
			"UPSTREAM_API_KEY":  "sk-test",
			"ADMIN_API_KEY":     "admin-secret-key-12345",
			"KEY_HMAC_SECRET":   "very-secure-hmac-secret-12345",
		}
		cfg, err := Load(func(k string) string { return env[k] })
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", v, err)
		}
		if cfg.UpstreamBaseURL != "https://example.com" {
			t.Errorf("for %s, expected base URL https://example.com, got %s", v, cfg.UpstreamBaseURL)
		}
		if cfg.UpstreamChatURL() != "https://example.com/v1/chat/completions" {
			t.Errorf("for %s, expected chat URL https://example.com/v1/chat/completions, got %s", v, cfg.UpstreamChatURL())
		}
		if cfg.UpstreamModelsURL() != "https://example.com/v1/models" {
			t.Errorf("for %s, expected models URL https://example.com/v1/models, got %s", v, cfg.UpstreamModelsURL())
		}
	}
}

func TestConfigFailClosed(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "missing upstream base url",
			env: map[string]string{
				"ADMIN_API_KEY":   "admin-key-123",
				"KEY_HMAC_SECRET": "hmac-secret-123456",
			},
		},
		{
			name: "invalid upstream base url scheme",
			env: map[string]string{
				"UPSTREAM_BASE_URL": "ftp://example.com",
				"ADMIN_API_KEY":     "admin-key-123",
				"KEY_HMAC_SECRET":   "hmac-secret-123456",
			},
		},
		{
			name: "missing upstream api key in bearer mode",
			env: map[string]string{
				"UPSTREAM_BASE_URL": "https://api.example.com",
				"ADMIN_API_KEY":     "admin-key-123",
				"KEY_HMAC_SECRET":   "hmac-secret-123456",
			},
		},
		{
			name: "short admin api key",
			env: map[string]string{
				"UPSTREAM_BASE_URL": "https://api.example.com",
				"UPSTREAM_API_KEY":  "sk-123",
				"ADMIN_API_KEY":     "short",
				"KEY_HMAC_SECRET":   "hmac-secret-123456",
			},
		},
		{
			name: "short hmac secret",
			env: map[string]string{
				"UPSTREAM_BASE_URL": "https://api.example.com",
				"UPSTREAM_API_KEY":  "sk-123",
				"ADMIN_API_KEY":     "admin-key-123456",
				"KEY_HMAC_SECRET":   "short",
			},
		},
		{
			name: "invalid static keys json",
			env: map[string]string{
				"UPSTREAM_BASE_URL":    "https://api.example.com",
				"UPSTREAM_API_KEY":     "sk-123",
				"ADMIN_API_KEY":        "admin-key-123456",
				"KEY_HMAC_SECRET":      "hmac-secret-123456",
				"DOWNSTREAM_KEYS_JSON": `{not valid json}`,
			},
		},
		{
			name: "duplicate static key id",
			env: map[string]string{
				"UPSTREAM_BASE_URL": "https://api.example.com",
				"UPSTREAM_API_KEY":  "sk-123",
				"ADMIN_API_KEY":     "admin-key-123456",
				"KEY_HMAC_SECRET":   "hmac-secret-123456",
				"DOWNSTREAM_KEYS_JSON": `[
					{"id": "k1", "key": "sk-1"},
					{"id": "k1", "key": "sk-2"}
				]`,
			},
		},
		{
			name: "invalid wrapper mode",
			env: map[string]string{
				"UPSTREAM_BASE_URL": "https://api.example.com",
				"UPSTREAM_API_KEY":  "sk-123",
				"ADMIN_API_KEY":     "admin-key-123456",
				"KEY_HMAC_SECRET":   "hmac-secret-123456",
				"WRAPPER_MODE":      "unknown_mode",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(func(k string) string { return tc.env[k] })
			if err == nil {
				t.Errorf("expected error for case %q, but got nil", tc.name)
			}
		})
	}
}
