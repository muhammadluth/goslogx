# Cardinality and privacy

Metric dimensions must have bounded values. Use fixed routes such as `/payments/:id`, known HTTP methods/statuses, configured database namespaces and enumerated business results. Resource identity belongs on the resource.

Do not use trace IDs, span IDs, request/transaction/user IDs, account numbers, emails, raw URLs, query strings, SQL statements, Redis keys, message payloads, raw errors or stack traces as metric dimensions. Even short strings can create unbounded series. Set SDK cardinality limits/views when accepting application-defined dimensions.

HTTP wrappers admit known method/status/protocol values. Body sizes remain measurements/span attributes, with no body-size metric dimension. Errors use the fixed `error.type=http_error`. Fiber admits registered route templates; net/http admits an explicit static `WithRoute` template. Static `WithMetricAttributes` admits only deployment environment and service version. Upstream labeler/header/URL/error detail data cannot bypass the HTTP attribute boundary. Configured routes, names and static labels remain caller-owned: use fixed values without secrets.

DB plugins allow enumerated operations and configured namespaces. Unknown operations become `QUERY`; they omit query text, parameters, credentials and raw errors. An error adds the bounded `error.type=database_error`. Redis disables statement/caller capture, uses fixed span names and strips raw error/address attributes. It uses a fixed pool label and records no key/value arguments.

Application-created spans and logs can carry useful identifiers under application policy. Goslogx log masking does not sanitize arbitrary external spans or metric dimensions. Prevent sensitive collection at the instrumentation boundary, and inspect upstream behavior again when upgrading dependencies.
