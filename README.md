# goslogx

[![Go Reference](https://pkg.go.dev/badge/github.com/muhammadluth/goslogx.svg)](https://pkg.go.dev/github.com/muhammadluth/goslogx)
[![Go Report Card](https://goreportcard.com/badge/github.com/muhammadluth/goslogx)](https://goreportcard.com/report/github.com/muhammadluth/goslogx)
[![Build Status](https://github.com/muhammadluth/goslogx/workflows/Go/badge.svg)](https://github.com/muhammadluth/goslogx/actions)
[![codecov](https://codecov.io/gh/muhammadluth/goslogx/graph/badge.svg?token=QD1YFY5MC8)](https://codecov.io/gh/muhammadluth/goslogx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**goslogx** is a high-performance, engineer-grade structured logging library for Go. Built on top of [Uber's Zap](https://github.com/uber-go/zap), it extends standard logging with zero-allocation field masking, compact stack trace formatting, and standardized DTOs for consistent observability across distributed systems.

## 🚀 Key Features

- **High-Performance Masking**: Zero-allocation field obfuscation using simple struct tags (`masked:"true"`).
- **Intelligent Stack Traces**: Re-formats multi-line stack traces into a compact, searchable single-line format.
- **Production-Ready DTOs**: Standardized schemas for HTTP, Database, and Message Queue interactions.
- **Zero-Allocation Design**: Leverages `sync.Pool` and byte-level scanning to minimize GC pressure.
- **Cloud-Native Severity**: Maps internal log levels to standard severity strings (DEBUG, INFO, WARNING, ERROR, CRITICAL).

## 📦 Installation

```bash
go get github.com/muhammadluth/goslogx
```

## 🛠️ Quick Start

### Basic Initialization
Initialize the global logger once in your `main()` or `init()`.

```go
package main

import "github.com/muhammadluth/goslogx"

func main() {
    // Initialize with service name and mask character (e.g., '*')
    // Use 0 to disable masking
    goslogx.SetupLog("payment-service", '*')

    goslogx.Info("trace-550e8400", "auth", goslogx.MESSSAGE_TYPE_EVENT, "user login successful", nil)
}
```

### Logging with Context
```go
traceID := "trace-123"

// Error with automatic stack trace
if err := processOrder(); err != nil {
    goslogx.Error(traceID, "order-worker", err)
}

// Warning with metadata
goslogx.Warning(traceID, "cache", "high latency detected", map[string]float64{"latency_ms": 450.5})
```

## 🛡️ Field Masking

Protect PII and sensitive data automatically. Fields tagged with `masked:"true"` are obfuscated based on their content:

- **Emails**: Shows first 2 characters + domain (e.g., `jo******@example.com`).
- **Short Secrets**: Fully masked if ≤ 8 characters.
- **Long Text**: Shows first 2 and last 2 characters (e.g., `ve****************12`).

```go
type UserData struct {
    Email    string `json:"email" masked:"true"`
    Password string `json:"password" masked:"true"`
    Address  string `json:"address"`
}

data := UserData{
    Email:    "john.doe@example.com",
    Password: "supersecret",
    Address:  "123 Go Lane",
}

// Password will be fully masked, Email partially, Address remains visible
goslogx.Info(traceID, "user-api", goslogx.MESSSAGE_TYPE_EVENT, "profile update", data)
```

## 📊 Standardized DTOs

Consistency is key for log aggregation. **goslogx** provides pre-defined DTOs:

| DTO | Category | Use Case |
|-----|----------|----------|
| `HTTPData` | Web | Logging Request/Response details and Latency. |
| `DBData` | Storage | Logging Queries, Drivers, and Execution Time. |
| `MQData` | Messaging | Logging Kafka/RabbitMQ Topic, MessageID, and Payload. |
| `GenericData` | Misc | Flexible schema for 3rd-party API integrations (Stripe, etc). |

## 🔍 Compact Stack Traces

Standard Zap stack traces are bulky. **goslogx** transforms them into a single-line readable format:

**Before:**
```text
goroutine 1 [running]:
main.main()
    /app/main.go:15 +0x12
```

**After (JSON output):**
```json
"stack_trace": "[goroutine 1 | main.main | /app/main.go:15]"
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.