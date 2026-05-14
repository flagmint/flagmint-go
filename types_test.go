package flagmint_test

import (
	"errors"
	"testing"

	flagmint "github.com/flagmint/flagmint-go"
)

// TestNewFeatureFlags creates a new FeatureFlags from raw data.
func TestNewFeatureFlags(t *testing.T) {
	raw := map[string]any{
		"bool-flag":   true,
		"string-flag": "value",
		"number-flag": float64(42),
		"json-flag":   map[string]any{"key": "value"},
	}

	flags := flagmint.NewFeatureFlags(raw)

	if flags.Len() != 4 {
		t.Errorf("Len: got %d, want 4", flags.Len())
	}

	if !flags.Has("bool-flag") {
		t.Error("Has: expected true for bool-flag")
	}

	if !flags.Has("string-flag") {
		t.Error("Has: expected true for string-flag")
	}
}

// TestFeatureFlags_Get retrieves flag values by key.
func TestFeatureFlags_Get(t *testing.T) {
	flags := flagmint.NewFeatureFlags(map[string]any{
		"feature": "enabled",
	})

	val, ok := flags.Get("feature")
	if !ok {
		t.Fatal("Get: expected ok=true")
	}
	if val != "enabled" {
		t.Errorf("Get: got %v, want 'enabled'", val)
	}

	_, ok = flags.Get("missing")
	if ok {
		t.Error("Get missing: expected ok=false")
	}
}

// TestFeatureFlags_Bool returns boolean flags with fallback support.
func TestFeatureFlags_Bool(t *testing.T) {
	flags := flagmint.NewFeatureFlags(map[string]any{
		"enabled":    true,
		"disabled":   false,
		"not-a-bool": "string",
	})

	if !flags.Bool("enabled", false) {
		t.Error("Bool: expected true for enabled")
	}

	if flags.Bool("disabled", true) {
		t.Error("Bool: expected false for disabled")
	}

	if flags.Bool("not-a-bool", true) != true {
		t.Error("Bool: expected fallback for non-bool value")
	}

	if flags.Bool("missing", true) != true {
		t.Error("Bool: expected fallback for missing key")
	}
}

// TestFeatureFlags_String returns string flags with fallback support.
func TestFeatureFlags_String(t *testing.T) {
	flags := flagmint.NewFeatureFlags(map[string]any{
		"message":      "hello",
		"not-a-string": 123,
	})

	if flags.String("message", "") != "hello" {
		t.Error("String: expected 'hello'")
	}

	if flags.String("not-a-string", "default") != "default" {
		t.Error("String: expected fallback for non-string")
	}

	if flags.String("missing", "default") != "default" {
		t.Error("String: expected fallback for missing key")
	}
}

// TestFeatureFlags_Float64 returns numeric flags with fallback support.
func TestFeatureFlags_Float64(t *testing.T) {
	flags := flagmint.NewFeatureFlags(map[string]any{
		"count":        float64(42),
		"not-a-number": "abc",
	})

	if flags.Float64("count", 0) != 42 {
		t.Error("Float64: expected 42")
	}

	if flags.Float64("not-a-number", 99) != 99 {
		t.Error("Float64: expected fallback for non-number")
	}

	if flags.Float64("missing", 99) != 99 {
		t.Error("Float64: expected fallback for missing key")
	}
}

// TestFeatureFlags_JSON unmarshals JSON flags into a target struct.
func TestFeatureFlags_JSON(t *testing.T) {
	type Config struct {
		Timeout int    `json:"timeout"`
		Host    string `json:"host"`
	}

	flags := flagmint.NewFeatureFlags(map[string]any{
		"config": map[string]any{
			"timeout": float64(30),
			"host":    "localhost",
		},
		"not-json": "string",
	})

	var cfg Config
	if err := flags.JSON("config", &cfg); err != nil {
		t.Fatalf("JSON: unexpected error: %v", err)
	}

	if cfg.Timeout != 30 {
		t.Errorf("JSON: Timeout = %d, want 30", cfg.Timeout)
	}
	if cfg.Host != "localhost" {
		t.Errorf("JSON: Host = %q, want localhost", cfg.Host)
	}

	// Test missing key
	if err := flags.JSON("missing", &cfg); !errors.Is(err, flagmint.ErrFlagNotFound) {
		t.Errorf("JSON missing: expected ErrFlagNotFound, got %v", err)
	}

	// Test non-JSON value
	if err := flags.JSON("not-json", &cfg); err == nil {
		t.Error("JSON: expected error for non-JSON value")
	}
}

// TestFeatureFlags_Clone creates a shallow copy of the flags.
func TestFeatureFlags_Clone(t *testing.T) {
	original := flagmint.NewFeatureFlags(map[string]any{
		"a": "1",
		"b": "2",
	})

	clone := original.Clone()

	if clone.Len() != original.Len() {
		t.Errorf("Clone: Len mismatch: got %d, want %d", clone.Len(), original.Len())
	}

	// Verify values match
	val1, ok1 := original.Get("a")
	val2, ok2 := clone.Get("a")
	if !ok1 || !ok2 || val1 != val2 {
		t.Error("Clone: values don't match")
	}
}

// TestFeatureFlags_Empty verifies empty flag set behavior.
func TestFeatureFlags_Empty(t *testing.T) {
	flags := flagmint.NewFeatureFlags(map[string]any{})

	if flags.Len() != 0 {
		t.Errorf("Empty flags Len: got %d, want 0", flags.Len())
	}

	if flags.Has("anything") {
		t.Error("Empty flags Has: expected false")
	}

	if flags.Bool("missing", true) != true {
		t.Error("Empty flags Bool: expected fallback")
	}
}

// TestErrFlagNotFound checks the error type.
func TestErrFlagNotFound(t *testing.T) {

	flags := flagmint.NewFeatureFlags(map[string]any{})

	var cfg struct{}
	err := flags.JSON("missing", &cfg)

	if !errors.Is(err, flagmint.ErrFlagNotFound) {
		t.Error("ErrFlagNotFound: expected error for missing flag")
	}
}
