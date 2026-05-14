// Package flagmint is the Go SDK for the Flagmint feature flag service.
//
// # Quick start
//
//	client, err := flagmint.NewClient("your-api-key",
//	    flagmint.WithContext(flagmint.EvaluationContext{
//	        Kind: "user",
//	        Key:  "user-123",
//	    }),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Block until flags are available (or timeout).
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	if err := client.Ready(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	enabled := client.BoolFlag("my-feature", false)
//
// # Architecture
//
// The SDK maintains a persistent connection to the Flagmint backend (WebSocket
// by default, with HTTP long-polling as a fallback).  Flags are evaluated
// server-side and streamed to the client in real time.  An optional local
// evaluator (Ticket 5) can evaluate flags without a round-trip when offline.
package flagmint
