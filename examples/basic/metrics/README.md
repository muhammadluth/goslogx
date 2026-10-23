# HTTP AND BUSINESS METRICS

This optional module uses the standard HTTP plugin for request telemetry and adds only an application-owned `orders.created` counter. Run `go test -race ./...` with Go 1.26. There is no second HTTP instrumentation stack or copied status writer. See [HTTP integrations](../../../docs/integrations/http/README.md).

## REQUIREMENTS

Go 1.26. No external service is required because the test uses in-memory telemetry.

## ENVIRONMENT VARIABLES

None.

## HOW TO RUN

```bash
cd examples/basic/metrics
go test -race ./...
```

## HOW TO GENERATE TRAFFIC

The test sends an in-memory HTTP request and increments the business counter.

## EXPECTED TELEMETRY AND VERIFICATION

The assertions verify HTTP server metrics plus `orders.created`, bounded attributes, and secret exclusion.

## HOW TO SHUT IT DOWN

The test closes its providers automatically.
