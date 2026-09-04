package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnvBasic(t *testing.T) {
	input := `
# Configuration file
UPSTREAM_BASE_URL=https://api.openai.com
UPSTREAM_API_KEY=sk-test-key-12345
PORT=9000
# Another comment
`
	res, err := ParseDotEnv(input)
	if err != nil {
		t.Fatalf("ParseDotEnv failed: %v", err)
	}

	if res["UPSTREAM_BASE_URL"] != "https://api.openai.com" {
		t.Errorf("expected https://api.openai.com, got %s", res["UPSTREAM_BASE_URL"])
	}
	if res["UPSTREAM_API_KEY"] != "sk-test-key-12345" {
		t.Errorf("expected sk-test-key-12345, got %s", res["UPSTREAM_API_KEY"])
	}
	if res["PORT"] != "9000" {
		t.Errorf("expected 9000, got %s", res["PORT"])
	}
}

func TestParseDotEnvQuotesAndEscapes(t *testing.T) {
	input := `
DOUBLE_QUOTED="hello world"
SINGLE_QUOTED='hello world single'
WITH_ESCAPES="line1\nline2\ttab\"quoted\""
JSON_STRING='[{"id":"client1","key":"sk-test"}]'
INLINE_COMMENT=foobar # this is a comment
EXPORTED=export_value
export EXPORTED2=export_value_2
`
	res, err := ParseDotEnv(input)
	if err != nil {
		t.Fatalf("ParseDotEnv failed: %v", err)
	}

	if res["DOUBLE_QUOTED"] != "hello world" {
		t.Errorf("expected hello world, got %q", res["DOUBLE_QUOTED"])
	}
	if res["SINGLE_QUOTED"] != "hello world single" {
		t.Errorf("expected hello world single, got %q", res["SINGLE_QUOTED"])
	}
	if res["WITH_ESCAPES"] != "line1\nline2\ttab\"quoted\"" {
		t.Errorf("expected unescaped string, got %q", res["WITH_ESCAPES"])
	}
	if res["JSON_STRING"] != `[{"id":"client1","key":"sk-test"}]` {
		t.Errorf("expected json string, got %q", res["JSON_STRING"])
	}
	if res["INLINE_COMMENT"] != "foobar" {
		t.Errorf("expected foobar, got %q", res["INLINE_COMMENT"])
	}
	if res["EXPORTED"] != "export_value" {
		t.Errorf("expected export_value, got %q", res["EXPORTED"])
	}
	if res["EXPORTED2"] != "export_value_2" {
		t.Errorf("expected export_value_2, got %q", res["EXPORTED2"])
	}
}

func TestParseDotEnvUTF8BOM(t *testing.T) {
	input := "\ufeffUPSTREAM_BASE_URL=https://newapi.chirei.de\nUPSTREAM_API_KEY=sk-test"
	res, err := ParseDotEnv(input)
	if err != nil {
		t.Fatalf("ParseDotEnv failed: %v", err)
	}
	if res["UPSTREAM_BASE_URL"] != "https://newapi.chirei.de" {
		t.Errorf("expected https://newapi.chirei.de, got %q", res["UPSTREAM_BASE_URL"])
	}
}

func TestParseDotEnvMultiline(t *testing.T) {
	input := `
MULTILINE_KEY="first line
second line
third line"
AFTER_KEY=valid
`
	res, err := ParseDotEnv(input)
	if err != nil {
		t.Fatalf("ParseDotEnv failed: %v", err)
	}

	expected := "first line\nsecond line\nthird line"
	if res["MULTILINE_KEY"] != expected {
		t.Errorf("expected %q, got %q", expected, res["MULTILINE_KEY"])
	}
	if res["AFTER_KEY"] != "valid" {
		t.Errorf("expected valid, got %q", res["AFTER_KEY"])
	}
}

func TestLoadDotEnvPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	err := os.WriteFile(envPath, []byte(`
TEST_DOTENV_KEY1=from_file
TEST_DOTENV_KEY2=from_file_2
`), 0644)
	if err != nil {
		t.Fatalf("failed to write temp .env: %v", err)
	}

	// Pre-set TEST_DOTENV_KEY1 in OS environment
	os.Setenv("TEST_DOTENV_KEY1", "from_system_env")
	os.Unsetenv("TEST_DOTENV_KEY2")
	defer func() {
		os.Unsetenv("TEST_DOTENV_KEY1")
		os.Unsetenv("TEST_DOTENV_KEY2")
	}()

	loadedPath, count, err := LoadDotEnv(envPath)
	if err != nil {
		t.Fatalf("LoadDotEnv failed: %v", err)
	}
	if loadedPath != envPath {
		t.Errorf("expected loadedPath %s, got %s", envPath, loadedPath)
	}
	if count != 1 {
		t.Errorf("expected 1 injected variable, got %d", count)
	}

	// System env must take precedence
	if val := os.Getenv("TEST_DOTENV_KEY1"); val != "from_system_env" {
		t.Errorf("expected system env to take precedence, got %s", val)
	}

	// Unset variable must be populated from .env
	if val := os.Getenv("TEST_DOTENV_KEY2"); val != "from_file_2" {
		t.Errorf("expected from_file_2, got %s", val)
	}
}

func TestLoadDotEnvBootstrapExample(t *testing.T) {
	tmpDir := t.TempDir()
	examplePath := filepath.Join(tmpDir, ".env.example")
	err := os.WriteFile(examplePath, []byte("BOOTSTRAP_KEY=bootstrap_value\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write .env.example: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to getwd: %v", err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	os.Unsetenv("BOOTSTRAP_KEY")
	os.Unsetenv("UPSTREAM_BASE_URL")
	defer func() {
		os.Unsetenv("BOOTSTRAP_KEY")
	}()

	loadedPath, count, err := LoadDotEnv()
	if err != nil {
		t.Fatalf("LoadDotEnv failed: %v", err)
	}
	if loadedPath == "" || count == 0 {
		t.Fatalf("expected bootstrapped .env, got path=%q count=%d", loadedPath, count)
	}
	if os.Getenv("BOOTSTRAP_KEY") != "bootstrap_value" {
		t.Errorf("expected bootstrap_value, got %s", os.Getenv("BOOTSTRAP_KEY"))
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".env")); err != nil {
		t.Errorf("expected .env file to be created: %v", err)
	}
}
