package flagmint_test

import (
	"context"
	"fmt"
	"log"
	"time"

	flagmint "github.com/flagmint/flagmint-go"
	"github.com/flagmint/flagmint-go/evaluate"
)

// Example_newClient demonstrates creating a client, waiting for it to be
// ready, and reading a boolean flag value.
func Example_newClient() {
	client, err := flagmint.NewClient("fm_sdk_your_api_key",
		flagmint.WithContext(flagmint.EvaluationContext{
			Kind: "user",
			Key:  "user-123",
			Attributes: map[string]any{
				"country": "DE",
				"plan":    "pro",
			},
		}),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ready blocks until the transport is connected and flags are available,
	// or until ctx is cancelled. In a real application, remove WithDeferInit()
	// to connect automatically on NewClient.
	_ = client.Ready(ctx)

	enabled := client.BoolFlag("dark-mode", false)
	fmt.Println("dark-mode:", enabled)
}

// Example_boolFlag shows how to evaluate a boolean feature flag with a
// default fallback value.
func Example_boolFlag() {
	client, err := flagmint.NewClient("fm_sdk_your_api_key",
		flagmint.WithDeferInit(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close() //nolint:errcheck

	// BoolFlag returns the flag value, or fallback (false) if the flag is
	// absent or not a boolean.
	if client.BoolFlag("new-checkout", false) {
		fmt.Println("new checkout experience is active")
	} else {
		fmt.Println("using standard checkout")
	}
	// Output:
	// using standard checkout
}

// Example_subscribe shows how to register a callback that fires whenever
// the flag set changes.
func Example_subscribe() {
	client, err := flagmint.NewClient("fm_sdk_your_api_key",
		flagmint.WithDeferInit(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close() //nolint:errcheck

	// Subscribe fires immediately with the current state, then again on every
	// update. The returned function removes the subscription.
	unsub := client.Subscribe(func(flags flagmint.FeatureFlags) {
		fmt.Println("flags updated, count:", flags.Len())
	})
	defer unsub()
	// Output:
	// flags updated, count: 0
}

// Example_withLocalEvaluation demonstrates setting up local evaluation with
// a manually supplied FlagConfig.  In production the configs come from the
// /evaluator/config endpoint.
func Example_withLocalEvaluation() {
	client, err := flagmint.NewClient("fm_sdk_your_api_key",
		flagmint.WithLocalEvaluation(),
		flagmint.WithContext(flagmint.EvaluationContext{
			Kind: "user",
			Key:  "user-456",
			Attributes: map[string]any{
				"plan": "pro",
			},
		}),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close() //nolint:errcheck

	darkMode := &evaluate.FlagConfig{
		Key:          "dark-mode",
		Type:         evaluate.FlagTypeBoolean,
		IsActive:     true,
		DefaultValue: false,
		Variations: []evaluate.Variation{
			{ID: "v-off", Value: false},
			{ID: "v-on", Value: true},
		},
		TargetingRules: []evaluate.TargetingRule{
			{
				ID:        "rule-pro",
				Kind:      "custom",
				LogicalOp: evaluate.LogicalAND,
				Conditions: []evaluate.Condition{
					{
						Attribute: "plan",
						Operator:  evaluate.OpEquals,
						Value:     "pro",
					},
				},
				VariationID: strPtr("v-on"),
			},
		},
	}
	darkMode.HydrateVariations()

	client.SetFlagConfigs(map[string]*evaluate.FlagConfig{
		"dark-mode": darkMode,
	})

	// Evaluation is fully local — no network round-trip.
	enabled := client.BoolFlag("dark-mode", false)
	fmt.Println("dark-mode:", enabled)
	// Output:
	// dark-mode: true
}

func strPtr(s string) *string { return &s }
