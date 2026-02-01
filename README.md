# goslogx

[![Go Reference](https://pkg.go.dev/badge/github.com/muhammadluth/goslogx.svg)](https://pkg.go.dev/github.com/muhammadluth/goslogx)
[![Go Report Card](https://goreportcard.com/badge/github.com/muhammadluth/goslogx)](https://goreportcard.com/report/github.com/muhammadluth/goslogx)
[![Build Status](https://github.com/muhammadluth/goslogx/workflows/Go/badge.svg)](https://github.com/muhammadluth/goslogx/actions)
[![codecov](https://codecov.io/gh/muhammadluth/goslogx/graph/badge.svg?token=QD1YFY5MC8)](https://codecov.io/gh/muhammadluth/goslogx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**goslogx** is a high-performance, production-ready structured logging library for Go. Built on top of [Uber's Zap](https://github.com/uber-go/zap), it provides automatic sensitive data masking, compact stack traces, and standardized DTOs for consistent observability across distributed systems.

## ✨ Key Features

- **🔐 Automatic Sensitive Data Masking**
  - Smart JSON body masking for HTTP requests/responses
  - Header masking for Authorization, API keys, tokens
  - Struct field masking with simple tags (`log:"masked:full"` or `log:"masked:partial"`)
  - Zero configuration required - works out of the box

- **⚡ High Performance**
  - Zero-allocation design using `sync.Pool` and cached reflection
  - **1-4 allocs/op** for most logging operations
  - **< 2µs/op** for simple logs
  - Efficient JSON masking with minimal overhead

- **📊 Standardized DTOs**
  - Pre-defined schemas for HTTP, Database, and Message Queue logging
  - Consistent field names across your entire infrastructure
  - Easy integration with log aggregation tools (ELK, Grafana, etc.)

- **🎯 Production-Ready**
  - Thread-safe singleton pattern with `sync.Once`
  - Compact single-line stack traces for better searchability
  - Cloud-native severity levels (DEBUG, INFO, WARNING, ERROR, CRITICAL)
  - **94%+ test coverage**

## 📦 Installation

```bash
go get github.com/muhammadluth/goslogx
```

**Requirements:** Go 1.21+

## 🚀 Quick Start

### Basic Usage

```go
package main

import (
    "github.com/muhammadluth/goslogx"
)

func main() {
    // Initialize logger (call once in main)
    goslogx.New(
        goslogx.WithServiceName("payment-service"),
        goslogx.WithMasking(true),
    )

    // Simple logging
    goslogx.Info(
        "trace-123",
        "api-handler",
        goslogx.MESSSAGE_TYPE_EVENT,
        "user login successful",
        nil,
    )

    // Error logging with stack trace
    if err := processPayment(); err != nil {
        goslogx.Error("trace-123", "payment-processor", err)
    }
}
```

### HTTP Request/Response Logging with Auto-Masking

```go
// Automatically masks sensitive fields in JSON body and headers
goslogx.Info(
    traceID,
    "api-gateway",
    goslogx.MESSSAGE_TYPE_REQUEST,
    "Incoming request",
    goslogx.HTTPData{
        Method:     "POST",
        URL:        "/api/v1/auth/login",
        StatusCode: 200,
        Headers: goslogx.MaskingLogHttpHeaders("headers", map[string][]string{
            "Authorization": {"Bearer eyJhbGci..."},  // Will be masked: "****"
            "Content-Type":  {"application/json"},    // Not masked
            "X-API-Key":     {"sk_live_123456"},      // Partially masked: "sk****56"
        }),
        Body: goslogx.MaskingLogJSONBytes("body", []byte(`{
            "username": "admin@example.com",  // Partially masked: "ad****om"
            "password": "secret123"           // Fully masked: "****"
        }`)),
        Duration: "125ms",
        ClientIP: "203.0.113.42",
    },
)
```

**Output:**
```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "msg": "Incoming request",
  "application_name": "api-gateway",
  "trace_id": "trace-123",
  "module": "api-gateway",
  "msg_type": "REQUEST",
  "severity": "INFO",
  "data": {
    "method": "POST",
    "url": "/api/v1/auth/login",
    "status_code": 200,
    "headers": {
      "Authorization": ["****"],
      "Content-Type": ["application/json"],
      "X-API-Key": ["sk****56"]
    },
    "body": "{\"username\":\"ad****om\",\"password\":\"****\"}",
    "duration": "125ms",
    "client_ip": "203.0.113.42"
  }
}
```

### Struct Field Masking

```go
type User struct {
    ID       string `json:"id"`
    Email    string `json:"email" log:"masked:partial"`
    Password string `json:"password" log:"masked:full"`
    Name     string `json:"name"`
}

user := User{
    ID:       "user-123",
    Email:    "john.doe@example.com",
    Password: "supersecret",
    Name:     "John Doe",
}

goslogx.Info(traceID, "user-service", goslogx.MESSSAGE_TYPE_EVENT, "user created", user)
// Email: "jo****om", Password: "****", Name: "John Doe" (unchanged)
```

## 🔐 Masking Strategies

### Automatic Field Detection

**Full Masking** (completely hidden as `****`):
- `password`, `passwd`, `pwd`
- `secret`, `secret_key`, `api_secret`
- `token`, `authorization`, `bearer`
- `credential`, `private_key`

**Partial Masking** (shows first/last 2 characters):
- `username`, `user_name`, `email`
- `phone`, `mobile`
- `api_key`, `access_key`, `client_id`

### Manual Masking Functions

```go
// Mask JSON string
masked := goslogx.MaskingLogJSONString("data", `{"password":"secret"}`)

// Mask JSON bytes
masked := goslogx.MaskingLogJSONBytes("body", jsonBytes)

// Mask HTTP headers
masked := goslogx.MaskingLogHttpHeaders("headers", headerMap)
```

## 📊 Standardized DTOs

### HTTPData
```go
goslogx.Info(traceID, "api", goslogx.MESSSAGE_TYPE_REQUEST, "request", goslogx.HTTPData{
    Method:     "GET",
    URL:        "/api/v1/users",
    StatusCode: 200,
    Headers:    headers,
    Body:       responseBody,
    Duration:   "45ms",
    ClientIP:   "192.168.1.1",
})
```

### DBData
```go
goslogx.Info(traceID, "database", goslogx.MESSSAGE_TYPE_IN, "query executed", goslogx.DBData{
    Driver:    "postgres",
    Operation: "SELECT",
    Database:  "users_db",
    Table:     "users",
    Statement: "SELECT * FROM users WHERE id = $1",
    Duration:  "12ms",
})
```

### MQData
```go
goslogx.Info(traceID, "messaging", goslogx.MESSSAGE_TYPE_IN, "message received", goslogx.MQData{
    Driver:    "kafka",
    Operation: "consume",
    Topic:     "user-events",
    Group:     "notification-service",
    MessageID: "msg-123",
    Payload:   eventData,
})
```

## ⚡ Performance Benchmarks

Benchmarks run on: `Intel Core i5-12400F @ 2.5GHz, 12 cores`

### Core Logging Operations

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `InfoNoData` | 780 ns | 1 | 24 B |
| `InfoWithDTO` | 1,946 ns | 8 | 296 B |
| `InfoWithNestedMasking` | 1,192 ns | 4 | 64 B |
| `InfoWithSliceMasking` | 1,667 ns | 12 | 288 B |
| `DebugNoData` | 108 ns | 4 | 184 B |
| `DebugWithDTO` | 173 ns | 4 | 184 B |
| `DebugWithNestedMasking` | 188 ns | 4 | 184 B |
| `DebugWithSliceMasking` | 201 ns | 5 | 208 B |
| `WarningNoData` | 1,582 ns | 6 | 449 B |
| `WarningWithDTO` | 3,084 ns | 14 | 770 B |
| `Error` | 2,041 ns | 7 | 465 B |

### JSON Masking Performance

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `MaskJSONString_Small` (< 100B) | 2,016 ns | 23 | 1,256 B |
| `MaskJSONString_Medium` (< 1KB) | 8,812 ns | 87 | 4,987 B |
| `MaskJSONString_Large` (> 1KB) | 17,639 ns | 193 | 9,815 B |
| `MaskHttpHeaders` | 1,438 ns | 13 | 560 B |

### Struct Masking Performance

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `StructMasking_Flat` | 1,146 ns | 4 | 56 B |
| `StructMasking_Nested` | 1,243 ns | 5 | 72 B |
| `StructMasking_Slice` | 1,549 ns | 12 | 240 B |

### HTTPData Logging

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `HTTPData_WithSensitiveData` | 2,151 ns | 10 | 489 B |
| `HTTPData_WithoutSensitiveData` | 1,850 ns | 8 | 417 B |

**Key Takeaways:**
- ✅ **Sub-microsecond** logging for simple operations
- ✅ **Single-digit allocations** for most use cases
- ✅ **Efficient masking** with minimal overhead (< 2µs for small JSON)
- ✅ **Scales well** with data size

Run benchmarks yourself:
```bash
go test -bench=. -benchmem -benchtime=1s
```

## 🎯 Configuration Options

```go
goslogx.New(
    // Set service/application name
    goslogx.WithServiceName("my-service"),
    
    // Enable/disable masking (default: true)
    goslogx.WithMasking(true),
    
    // Set log level (default: Info)
    goslogx.WithDebug(true),  // Enables Debug level
    
    // Custom output writer (default: os.Stdout)
    goslogx.WithOutput(customWriter),
)
```

## 📖 API Documentation

Full API documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/muhammadluth/goslogx).

### Core Functions

- `New(...Option)` - Initialize logger with options
- `Info(traceID, module, msgType, msg, data)` - Log informational messages
- `Debug(traceID, module, msgType, msg, data)` - Log debug messages
- `Warning(traceID, module, msg, data)` - Log warnings
- `Error(traceID, module, err)` - Log errors with stack trace
- `Fatal(traceID, module, err)` - Log fatal errors and exit

### Masking Functions

- `MaskingLogJSONString(key, jsonStr)` - Mask sensitive fields in JSON string
- `MaskingLogJSONBytes(key, jsonBytes)` - Mask sensitive fields in JSON bytes
- `MaskingLogHttpHeaders(key, headers)` - Mask sensitive HTTP headers

## 🧪 Testing

```bash
# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Current Coverage:** 94.1%

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 🙏 Acknowledgments

- Built on top of [Uber's Zap](https://github.com/uber-go/zap) - blazing fast, structured logging
- Inspired by production needs in high-scale distributed systems

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

**Made with ❤️ for the Go community**