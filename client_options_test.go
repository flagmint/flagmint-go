package flagmint_test

import (
	"os"
	"testing"

	flagmint "github.com/flagmint/flagmint-go"
)

// TestWithContext sets the evaluation context option.
func TestWithContext(t *testing.T) {
	ctx := flagmint.EvaluationContext{
		Kind: "user",
		Key:  "user-123",
		Attributes: map[string]any{
			"email": "test@example.com",
		},
	}

	c, err := flagmint.NewClient("api-key",
		flagmint.WithContext(ctx),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestWithTransportMode selects the transport mechanism.
func TestWithTransportMode(t *testing.T) {
	c, err := flagmint.NewClient("api-key",
		flagmint.WithTransportMode(flagmint.TransportLongPolling),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestWithCache enables or disables caching.
func TestWithCache(t *testing.T) {
	c, err := flagmint.NewClient("api-key",
		flagmint.WithCache(true),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("NewClient with cache: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestWithDeferInit defers client initialization.
func TestWithDeferInit(t *testing.T) {
	c, err := flagmint.NewClient("api-key",
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close() //nolint:errcheck

	// Flags should be empty before Ready is called
	if c.GetFlags().Len() != 0 {
		t.Error("Flags should be empty before Ready")
	}
}

// TestWithEndpoints overrides the default endpoints.
func TestWithEndpoints(t *testing.T) {
	c, err := flagmint.NewClient("api-key",
		flagmint.WithEndpoints(
			"https://custom-api.example.com",
			"wss://custom-api.example.com",
		),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("NewClient with custom endpoints: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestEnvVar_FlagmintEnv tests environment selection via FLAGMINT_ENV.
func TestEnvVar_FlagmintEnv(t *testing.T) {
	t.Run("staging", func(t *testing.T) {
		// Save original env
		original := os.Getenv(flagmint.EnvFlagmintEnv)
		defer os.Setenv(flagmint.EnvFlagmintEnv, original)

		os.Setenv(flagmint.EnvFlagmintEnv, "staging")

		c, err := flagmint.NewClient("api-key", flagmint.WithDeferInit())
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer c.Close() //nolint:errcheck
	})

	t.Run("local", func(t *testing.T) {
		original := os.Getenv(flagmint.EnvFlagmintEnv)
		defer os.Setenv(flagmint.EnvFlagmintEnv, original)

		os.Setenv(flagmint.EnvFlagmintEnv, "local")

		c, err := flagmint.NewClient("api-key", flagmint.WithDeferInit())
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer c.Close() //nolint:errcheck
	})

	t.Run("prod", func(t *testing.T) {
		original := os.Getenv(flagmint.EnvFlagmintEnv)
		defer os.Setenv(flagmint.EnvFlagmintEnv, original)

		os.Setenv(flagmint.EnvFlagmintEnv, "prod")

		c, err := flagmint.NewClient("api-key", flagmint.WithDeferInit())
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer c.Close() //nolint:errcheck
	})
}

// TestEnvVar_ExplicitEndpoints tests explicit endpoint environment variables.
func TestEnvVar_ExplicitEndpoints(t *testing.T) {
	// Save originals
	origRest := os.Getenv(flagmint.EnvFlagmintRestEndpoint)
	origWS := os.Getenv(flagmint.EnvFlagmintWSEndpoint)
	defer func() {
		os.Setenv(flagmint.EnvFlagmintRestEndpoint, origRest)
		os.Setenv(flagmint.EnvFlagmintWSEndpoint, origWS)
	}()

	// Set explicit endpoints
	os.Setenv(flagmint.EnvFlagmintRestEndpoint, "https://my-api.example.com")
	os.Setenv(flagmint.EnvFlagmintWSEndpoint, "wss://my-api.example.com")

	c, err := flagmint.NewClient("api-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatalf("NewClient with env endpoints: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestEnvVar_Precedence verifies that explicit endpoints override FLAGMINT_ENV.
func TestEnvVar_Precedence(t *testing.T) {
	origRest := os.Getenv(flagmint.EnvFlagmintRestEndpoint)
	origEnv := os.Getenv(flagmint.EnvFlagmintEnv)
	defer func() {
		os.Setenv(flagmint.EnvFlagmintRestEndpoint, origRest)
		os.Setenv(flagmint.EnvFlagmintEnv, origEnv)
	}()

	// Set both env and explicit endpoint
	os.Setenv(flagmint.EnvFlagmintEnv, "staging")
	os.Setenv(flagmint.EnvFlagmintRestEndpoint, "https://explicit-api.example.com")

	// Explicit endpoint should take precedence
	c, err := flagmint.NewClient("api-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestEnvVar_UnknownEnv falls back to production when unknown env is set.
func TestEnvVar_UnknownEnv(t *testing.T) {
	origEnv := os.Getenv(flagmint.EnvFlagmintEnv)
	defer os.Setenv(flagmint.EnvFlagmintEnv, origEnv)

	os.Setenv(flagmint.EnvFlagmintEnv, "unknown-env")

	// Should not error, falls back to production
	c, err := flagmint.NewClient("api-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatalf("NewClient with unknown env: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestEnvVar_Empty falls back to production when env vars are empty.
func TestEnvVar_Empty(t *testing.T) {
	origRest := os.Getenv(flagmint.EnvFlagmintRestEndpoint)
	origEnv := os.Getenv(flagmint.EnvFlagmintEnv)
	defer func() {
		os.Setenv(flagmint.EnvFlagmintRestEndpoint, origRest)
		os.Setenv(flagmint.EnvFlagmintEnv, origEnv)
	}()

	// Clear env vars
	os.Unsetenv(flagmint.EnvFlagmintRestEndpoint)
	os.Unsetenv(flagmint.EnvFlagmintEnv)

	// Should use production defaults
	c, err := flagmint.NewClient("api-key", flagmint.WithDeferInit())
	if err != nil {
		t.Fatalf("NewClient with no env vars: %v", err)
	}
	defer c.Close() //nolint:errcheck
}

// TestMultipleOptions combines multiple configuration options.
func TestMultipleOptions(t *testing.T) {
	ctx := flagmint.EvaluationContext{Kind: "user", Key: "u1"}

	c, err := flagmint.NewClient("api-key",
		flagmint.WithContext(ctx),
		flagmint.WithTransportMode(flagmint.TransportLongPolling),
		flagmint.WithCache(true),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		t.Fatalf("NewClient with multiple options: %v", err)
	}
	defer c.Close() //nolint:errcheck
}
