# SQL DRIVER COMPATIBILITY

Run `go test ./...` in this module to check generic SQL instrumentation with SQLite, PostgreSQL, MySQL and SQL Server driver contracts. The SQLite path runs locally; optional database integration cases use their configured test endpoints. See the [SQL plugin guide](../../../../docs/integrations/database/sql.md).

## REQUIREMENTS AND ENVIRONMENT VARIABLES

SQLite tests need only Go. Optional live checks read the driver-specific DSN variables defined in `integration_test.go`; unset endpoints are skipped.

## HOW TO RUN

```bash
cd examples/database/sql/compatibility
go test ./...
```

## EXPECTED TELEMETRY AND VERIFICATION

Passing tests confirm driver construction, operations, privacy, and generic instrumentation compatibility. Tests close all pools and providers automatically.

## HOW TO SHUT IT DOWN

No manual shutdown is required; the Go test process closes its pools and telemetry providers.
