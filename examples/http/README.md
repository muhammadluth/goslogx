# HTTP examples

Each framework has its own runnable server example and a downstream integration test. Run `go run .` within `std`, `fiber`, `gin`, `echo`, `chi`, or `gorillamux`; each serves `/health` on port 8080. Run `go test ./...` in an example module to verify trace propagation, log correlation and privacy across HTTP, Bun, Redis and NATS. The shared test fixture lives under `internal/testkit` and is not a plugin.

The `masking` example demonstrates logging of masked HTTP request and response data.
