// Package main demonstrates basic usage of the Flagmint Go SDK.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flagmint/flagmint-go"
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

	ctx2 := flagmint.EvaluationContext{
		Kind: "user",
		Key:  "user-456",
		Attributes: map[string]any{
			"email": "alice@gmail.com",
		},
	}
	client, err := flagmint.NewClient(token,
		flagmint.WithContext(ctx2),
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

	// Wait for the client to be ready (fetch initial flags).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := client.Ready(ctx); err != nil {
		log.Printf("client not ready: %v (will use fallback values)", err)
	}
	cancel()

	flags := client.GetFlags()

	fmt.Printf("fetched %d flags\n", flags.Len())

	// Type-safe flag retrieval — no type assertions needed.
	isNewUI := flags.Bool("my_feature", false)
	maxRetries := flags.Bool("max_retries", true)
	welcomeMsg := flags.String("welcome_message", "Welcome!")

	fmt.Printf("my_feature      = %v\n", isNewUI)
	fmt.Printf("max_retries     = %v\n", maxRetries)
	fmt.Printf("welcome-message = %v\n", welcomeMsg)

	// client.UpdateContext(ctx2)

	// Wait for flags to update after context change using Subscribe
	firstDelivery := true
	flagsCh := make(chan flagmint.FeatureFlags, 1)
	unsub := client.Subscribe(func(flags flagmint.FeatureFlags) {
		if firstDelivery {
			firstDelivery = false
			return
		}
		select {
		case flagsCh <- flags:
		default:
		}
	})
	defer unsub()

	client.UpdateContext(ctx2)

	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
	select {
	case updatedFlags := <-flagsCh:
		fmt.Printf("\nAfter context change - fetched %d flags\n", updatedFlags.Len())

		isNewUI = updatedFlags.Bool("my_feature", false)
		maxRetries = updatedFlags.Bool("max_retries", true)
		welcomeMsg = updatedFlags.String("welcome_message", "Welcome!")
		// Convenience methods directly on the client.
		fmt.Printf("my_feature_2      = %v\n", isNewUI)
		fmt.Printf("max_retries_2     = %v\n", maxRetries)
		fmt.Printf("welcome-message_2 = %v\n", welcomeMsg)
	case <-ctx.Done():
		log.Printf("Timeout waiting for flags update: %v", ctx.Err())
	}
	cancel()

	// Block until OS signal (Ctrl+C, SIGTERM from Docker/k8s)
	fmt.Println("\n🔌 WebSocket connection is active. Press Ctrl+C to close.")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
}
