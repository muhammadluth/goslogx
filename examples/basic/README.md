# BASIC EXAMPLES

[Metrics](metrics/README.md) adds an application counter alongside the standard HTTP plugin. For logging-only setup and installation, see the [root README](../../README.md).

## REQUIREMENTS

Use the Go version declared by the selected nested module and install any service named by that example.

## ENVIRONMENT VARIABLES

Open the selected example README for its service-specific variables. Examples without variables use their documented localhost defaults.

## HOW TO RUN

Change into one of the linked example directories, then run `go run .` for executable examples or `go test ./...` for test-only modules.

## HOW TO GENERATE TRAFFIC

Use the command in the selected example README. Snippet-only examples are intended to be copied into an application and do not start a server.

## EXPECTED TELEMETRY

The selected README identifies its spans, metrics, logs, and propagation behavior.

## HOW TO VERIFY IT

Run the nested module tests where present and inspect the configured Collector/backend.

## HOW TO SHUT IT DOWN

Stop traffic and clients before flushing and shutting down the logger.
