module github.com/flagmint/flagmint-go

go 1.25.0

require (
	github.com/coder/websocket v1.8.14
	github.com/joho/godotenv v1.5.1
	go.uber.org/goleak v1.3.0 // used for test goroutine leak detection
)
