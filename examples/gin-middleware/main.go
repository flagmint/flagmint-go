//go:build ignore

// Package main demonstrates a feature flag middleware for the Gin HTTP
// framework using the Flagmint Go SDK.
//
// Each request carries its own EvaluationContext (user ID + attributes
// extracted from headers / JWT claims).  The middleware stores the per-request
// client in the Gin context so handlers can read flags without any additional
// setup.
//
// Run:
//
//	go run main.go
//
// Then test with:
//
//	curl http://localhost:8080/dashboard
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	flagmint "github.com/flagmint/flagmint-go"
	"github.com/flagmint/flagmint-go/examples/util"
	"github.com/joho/godotenv"
)

const (
	flagmintClientKey = "flagmintClient"
	apiKey            = "fm_sdk_your_api_key"
)

// FlagmintMiddleware builds a per-request EvaluationContext from request
// headers and attaches a ready FlagClient to the Gin context.
//
// In a production application, extract the user ID and attributes from a
// decoded JWT or session instead of reading raw headers.
func FlagmintMiddleware(sharedClient *flagmint.FlagClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			userID = "anonymous"
		}
		plan := c.GetHeader("X-User-Plan")
		if plan == "" {
			plan = "free"
		}

		// Update the shared client's evaluation context for this request.
		// UpdateContext is goroutine-safe.
		_ = sharedClient.UpdateContext(flagmint.EvaluationContext{
			Kind: "user",
			Key:  userID,
			Attributes: map[string]any{
				"plan": plan,
			},
		})

		// Make the client available to downstream handlers.
		c.Set(flagmintClientKey, sharedClient)
		c.Next()
	}
}

// flagsFromContext is a helper used by handlers to retrieve the FlagClient
// stored by FlagmintMiddleware.
func flagsFromContext(c *gin.Context) *flagmint.FlagClient {
	if v, ok := c.Get(flagmintClientKey); ok {
		if client, ok := v.(*flagmint.FlagClient); ok {
			return client
		}
	}
	return nil
}

func main() {
	_ = godotenv.Load("../../.env")

	token := os.Getenv("FLAGMINT_SDK_TOKEN")
	if token == "" {
		token = "ksjdklj"
	}
	fmt.Printf("Using SDK token: %s\n", util.MaskToken(token))
	// Create a single shared client for the lifetime of the server.
	client, err := flagmint.NewClient(token,
		flagmint.WithOnError(func(err error) {
			log.Printf("flagmint error: %v", err)
		}),
	)
	if err != nil {
		log.Fatal("failed to create flagmint client:", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("flagmint close error: %v", err)
		}
	}()

	// Wait for flags to be ready before serving traffic.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ready(ctx); err != nil {
		log.Printf("flagmint not ready (using defaults): %v", err)
	}

	r := gin.Default()
	r.Use(FlagmintMiddleware(client))

	r.GET("/dashboard", func(c *gin.Context) {
		fc := flagsFromContext(c)

		response := gin.H{
			"page": "dashboard",
		}

		if fc != nil {
			response["newDashboard"] = fc.BoolFlag("new-dashboard", false)
			response["maxWidgets"] = fc.NumberFlag("max-widgets", 10)
			response["theme"] = fc.StringFlag("dashboard-theme", "light")
		}

		c.JSON(http.StatusOK, response)
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	log.Println("listening on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
