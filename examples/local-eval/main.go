// Package main demonstrates local flag evaluation with the Flagmint Go SDK.
package main

import (
	"fmt"
	"log"
	"os"

	flagmint "github.com/flagmint/flagmint-go"
	"github.com/flagmint/flagmint-go/evaluate"
	"github.com/flagmint/flagmint-go/examples/util"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file (if it exists)
	_ = godotenv.Load()

	token := os.Getenv("FLAGMINT_SDK_TOKEN")
	if token == "" {
		token = "demo-api-key"
	}
	fmt.Printf("Using SDK token: %s\n", util.MaskToken(token))
	// Create a client with local evaluation enabled (no transport connection
	// needed when using manually supplied flag configs).
	client, err := flagmint.NewClient(token,
		flagmint.WithContext(flagmint.EvaluationContext{
			Kind: "user",
			Key:  "user-456",
			Attributes: map[string]any{
				"plan":    "pro",
				"country": "DE",
			},
		}),
		flagmint.WithLocalEvaluation(),
		flagmint.WithDeferInit(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	// Build flag configurations (in production these come from the server via
	// the /evaluator/config endpoint).
	darkMode := &evaluate.FlagConfig{
		Key:          "dark-mode",
		Type:         evaluate.FlagTypeBoolean,
		IsActive:     true,
		DefaultValue: false,
		Variations: []evaluate.Variation{
			{ID: "v-false", Value: false},
			{ID: "v-true", Value: true},
		},
		TargetingRules: []evaluate.TargetingRule{
			{
				ID:         "rule-pro",
				Kind:       "custom",
				OrderIndex: 1,
				LogicalOp:  evaluate.LogicalAND,
				Conditions: []evaluate.Condition{
					{
						Attribute: "plan",
						Operator:  evaluate.OpEquals,
						Value:     "pro",
					},
				},
				VariationID: strPtr("v-true"),
			},
		},
	}
	darkMode.HydrateVariations()

	uploadLimit := &evaluate.FlagConfig{
		Key:          "max-upload-mb",
		Type:         evaluate.FlagTypeNumber,
		IsActive:     true,
		DefaultValue: float64(10),
		Variations: []evaluate.Variation{
			{ID: "v-50", Value: float64(50)},
		},
		TargetingRules: []evaluate.TargetingRule{
			{
				ID:          "rule-all",
				Kind:        "custom",
				OrderIndex:  1,
				LogicalOp:   evaluate.LogicalAND,
				Conditions:  []evaluate.Condition{},
				VariationID: strPtr("v-50"),
			},
		},
	}
	uploadLimit.HydrateVariations()

	client.SetFlagConfigs(map[string]*evaluate.FlagConfig{
		"dark-mode":     darkMode,
		"max-upload-mb": uploadLimit,
	})

	for _, key := range []string{"dark-mode", "max-upload-mb", "unknown-flag"} {
		val := client.GetFlag(key, "<not found>")
		fmt.Printf("%-20s = %v\n", key, val)
	}
}

func strPtr(s string) *string { return &s }
