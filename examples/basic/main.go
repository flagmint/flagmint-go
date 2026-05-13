// Package main demonstrates basic usage of the Flagmint Go SDK.
package main

import (
	"fmt"
	"log"

	flagmint "github.com/flagmint/flagmint-go"
)

func main() {
	client, err := flagmint.NewClient("demo-api-key",
		flagmint.WithContext(flagmint.EvaluationContext{
			Kind: "user",
			Key:  "user-123",
			Attributes: map[string]any{
				"email": "alice@example.com",
				"plan":  "pro",
			},
		}),
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

	flags := client.GetFlags()
	fmt.Printf("fetched %d flags\n", flags.Len())

	// Type-safe flag retrieval — no type assertions needed.
	isNewUI := flags.Bool("my-feature", false)
	maxRetries := flags.Float64("max-retries", 3.0)
	welcomeMsg := flags.String("welcome-message", "Welcome!")

	fmt.Printf("my-feature      = %v\n", isNewUI)
	fmt.Printf("max-retries     = %v\n", maxRetries)
	fmt.Printf("welcome-message = %v\n", welcomeMsg)

	// Convenience methods directly on the client.
	fmt.Printf("dark-mode       = %v\n", client.Bool("dark-mode", false))
}
