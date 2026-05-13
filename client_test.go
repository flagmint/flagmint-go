package flagmint_test

import (
	"errors"
	"testing"

	flagmint "github.com/flagmint/flagmint-go"
)

func TestNewClient_EmptyAPIKey(t *testing.T) {
	_, err := flagmint.NewClient("")
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
}

func TestNewClient_DeferInit(t *testing.T) {
	c, err := flagmint.NewClient("test-key",
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer c.Close() //nolint:errcheck

	// No flags should be available before Initialize.
	if c.GetFlags().Len() != 0 {
		t.Fatalf("expected empty flags, got %d", c.GetFlags().Len())
	}

	if c.GetFlags().Has("absent") {
		t.Fatal("expected missing flag to return Has=false")
	}
}

func TestSetContext(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	// Should not panic.
	c.SetContext(flagmint.EvaluationContext{Kind: "user", Key: "u999"})
}

func TestWithContext_Option(t *testing.T) {
	ctx := flagmint.EvaluationContext{Kind: "user", Key: "u1"}
	c, err := flagmint.NewClient("test-key",
		flagmint.WithContext(ctx),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck
}

func TestFeatureFlags_TypedGetters(t *testing.T) {
	flags := flagmint.NewFeatureFlags(map[string]any{
		"dark-mode":   true,
		"retries":     float64(3),
		"greeting":    "hello",
		"config":      map[string]any{"timeout": float64(30)},
	})

	// Bool
	if !flags.Bool("dark-mode", false) {
		t.Error("Bool: expected true")
	}
	if flags.Bool("missing", true) != true {
		t.Error("Bool fallback: expected true")
	}

	// Float64
	if flags.Float64("retries", 0) != 3 {
		t.Errorf("Float64: got %v, want 3", flags.Float64("retries", 0))
	}
	if flags.Float64("missing", 99) != 99 {
		t.Errorf("Float64 fallback: got %v, want 99", flags.Float64("missing", 99))
	}

	// String
	if flags.String("greeting", "") != "hello" {
		t.Errorf("String: got %q, want hello", flags.String("greeting", ""))
	}
	if flags.String("missing", "default") != "default" {
		t.Errorf("String fallback: got %q, want default", flags.String("missing", "default"))
	}

	// JSON
	var cfg struct {
		Timeout float64 `json:"timeout"`
	}
	if err := flags.JSON("config", &cfg); err != nil {
		t.Fatalf("JSON: unexpected error: %v", err)
	}
	if cfg.Timeout != 30 {
		t.Errorf("JSON: Timeout = %v, want 30", cfg.Timeout)
	}

	// JSON missing key
	if err := flags.JSON("missing", &cfg); !errors.Is(err, flagmint.ErrFlagNotFound) {
		t.Errorf("JSON missing: expected ErrFlagNotFound, got %v", err)
	}

	// Has / Len
	if !flags.Has("dark-mode") {
		t.Error("Has: expected true for existing key")
	}
	if flags.Has("missing") {
		t.Error("Has: expected false for missing key")
	}
	if flags.Len() != 4 {
		t.Errorf("Len: got %d, want 4", flags.Len())
	}
}

func TestFlagClient_TypedConvenience(t *testing.T) {
	c, err := flagmint.NewClient("test-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck

	// All fallbacks before flags are loaded.
	if c.Bool("x", true) != true {
		t.Error("Bool convenience fallback")
	}
	if c.String("x", "fb") != "fb" {
		t.Error("String convenience fallback")
	}
	if c.Float64("x", 7) != 7 {
		t.Error("Float64 convenience fallback")
	}
	if err := c.JSON("x", new(struct{})); !errors.Is(err, flagmint.ErrFlagNotFound) {
		t.Errorf("JSON convenience fallback: %v", err)
	}
}
