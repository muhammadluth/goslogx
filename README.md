# GOSLOGX

[![Go Reference](https://pkg.go.dev/badge/github.com/muhammadluth/goslogx.svg)](https://pkg.go.dev/github.com/muhammadluth/goslogx)
[![Go Report Card](https://goreportcard.com/badge/github.com/muhammadluth/goslogx)](https://goreportcard.com/report/github.com/muhammadluth/goslogx)
[![Build Status](https://github.com/muhammadluth/goslogx/workflows/Go/badge.svg)](https://github.com/muhammadluth/goslogx/actions)
[![codecov](https://codecov.io/gh/muhammadluth/goslogx/branch/master/graph/badge.svg)](https://codecov.io/gh/muhammadluth/goslogx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**GOSLOGX** is a high-performance, structured logging library for Go, built on top of [Uber's Zap](https://github.com/uber-go/zap). It provides a standardized logging format with specialized Data Transfer Objects (DTOs) for common application scenarios like HTTP requests, database operations, and message queue events.

## Features

-   🚀 **High Performance**: Powered by Zap's production-optimized configuration.
-   📝 **Structured JSON Logging**: Consistent JSON output for easy parsing and observability.
-   📦 **Standardized DTOs**: Pre-defined structs for `HTTP`, `DB`, `MQ`, and `Generic` events.
-   🏷️ **Rich Metadata**: Automatically includes `trace_id`, `module`, `severity`, and source code location.
-   🛠️ **Easy Migration**: Simple API compatible with common logging patterns.

## Installation

```bash
go get github.com/muhammadluth/goslogx
```

## Usage

### 1. Setup

Initialize the logger once at the start of your application, for example in `main.go`:

```go
package main

import (
	"github.com/muhammadluth/goslogx"
)

func main() {
	// Initialize with your service name
	goslogx.SetupLog("my-service-name")
	
	// ... your app code
}
```

### 2. Basic Logging

Use the standard logging functions `Info`, `Debug`, `Warning`, `Error`, and `Fatal`.

```go
ctx := context.Background()
traceID := "trace-123"

// Simple Info log
goslogx.Info(ctx, traceID, "user-module", goslogx.MESSSAGE_TYPE_IN, "processing user", nil)

// Warning with data
goslogx.Warning(ctx, traceID, "payment", "payment gateway timeout", map[string]int{"attempt": 3})

// Error logging (includes stack trace automatically)
err := errors.New("database connection failed")
goslogx.Error(ctx, traceID, "db", err)
```

### 3. Using DTOs

Use the standardized DTOs to log detailed context for specific operations.

#### HTTP Request

```go
reqData := goslogx.HTTPRequestData{
    Method:     "GET",
    URL:        "/api/users/123",
    StatusCode: 200,
    ClientIP:   "192.168.1.1",
}

goslogx.Info(ctx, traceID, "http-handler", goslogx.MESSSAGE_TYPE_IN, "request handled", reqData)
```

#### Database Operation

```go
dbData := goslogx.DBData{
    Driver:     "postgres",
    Operation:  "SELECT",
    Table:      "users",
    Statement:  "SELECT * FROM users WHERE id = $1",
    DurationMs: 45,
}

goslogx.Info(ctx, traceID, "user-repo", goslogx.MESSSAGE_TYPE_OUT, "query executed", dbData)
```

#### Message Queue

```go
mqData := goslogx.MQData{
    Driver:    "kafka",
    Operation: "consume",
    Topic:     "order-events",
    Group:     "order-service",
    MessageID: "msg-uuid",
}

goslogx.Info(ctx, traceID, "event-consumer", goslogx.MESSSAGE_TYPE_IN, "event received", mqData)
```

#### Generic / Third-Party

```go
extData := goslogx.GenericData{
    Service: "Stripe",
    Action:  "Charge",
    Payload: map[string]interface{}{"amount": 2000, "currency": "idr"},
}

goslogx.Info(ctx, traceID, "payment-service", goslogx.MESSSAGE_TYPE_OUT, "calling external api", extData)
```

## Output Format

Logs are output in JSON format:

```json
{
  "level": "info",
  "time": "2024-01-09T10:00:00Z",
  "application_name": "my-service-name",
  "trace_id": "trace-123",
  "module": "user-repo",
  "msg_type": "OUT",
  "severity": "INFO",
  "msg": "query executed",
  "data": {
    "driver": "postgres",
    "operation": "SELECT",
    "table": "users",
    "statement": "SELECT * FROM users WHERE id = $1",
    "duration_ms": 45
  }
}
```

## License

MIT