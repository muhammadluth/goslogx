# UPSTREAM INSTRUMENTATION

## CHOOSE AN INTEGRATION

Each integration has one canonical module under `github.com/muhammadluth/goslogx/plugins/`.
Install the module for your client; core does not import these frameworks or drivers.
The links below point to the instrumentation source or, for native wrappers, the client API.
Each plugin's `go.mod` pins the version used by that module.

| Library | Plugin suffix | Implementation / source |
| --- | --- | --- |
| HTTP std | `http/std` | [otelhttp](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http/otelhttp) |
| Fiber | `http/fiber` | [Fiber contrib v3/otel](https://github.com/gofiber/contrib/tree/main/v3/otel) |
| Gin | `http/gin` | [otelgin](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/github.com/gin-gonic/gin/otelgin) |
| Echo | `http/echo` | [otelecho](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/github.com/labstack/echo/otelecho) |
| Chi | `http/chi` | [std adapter / otelhttp](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http/otelhttp) |
| Gorilla Mux | `http/gorillamux` | [otelmux](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/github.com/gorilla/mux/otelmux) |
| httprouter | `http/httprouter` | [std adapter / otelhttp](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http/otelhttp) |
| go-restful | `http/gorestful` | [otelrestful plus native duration measurement](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/github.com/emicklei/go-restful/otelrestful) |
| fasthttp | `http/fasthttp` | [native handler wrapper](https://github.com/valyala/fasthttp) |
| gRPC | `grpc` | [otelgrpc](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/google.golang.org/grpc/otelgrpc) |
| Bun | `database/bun` | [bunotel](https://github.com/uptrace/bun/tree/master/extra/bunotel) |
| Cassandra / Scylla | `database/cassandra` | [native gocql observers](https://github.com/apache/cassandra-gocql-driver) |
| ClickHouse | `database/clickhouse` | [native connection/rows/batch wrapper](https://github.com/ClickHouse/clickhouse-go) |
| DynamoDB | `database/dynamodb` | [otelaws](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws) |
| Elasticsearch | `database/elasticsearch` | [std adapter / otelhttp](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http/otelhttp) |
| Ent | `database/ent` | [native dialect driver wrapper](https://github.com/ent/ent) |
| GORM | `database/gorm` | [GORM OpenTelemetry callbacks](https://github.com/go-gorm/opentelemetry) |
| MongoDB v2 | `database/mongo` | [otelmongo plus driver pool events](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo) |
| Neo4j v6 | `database/neo4j` | [native session/transaction/result wrapper](https://github.com/neo4j/neo4j-go-driver) |
| OpenSearch | `database/opensearch` | [std adapter / otelhttp](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http/otelhttp) |
| go-pg | `database/pg` | [native QueryHook; pgotel reference](https://github.com/go-pg/pg/tree/v10/extra/pgotel) |
| pgx v5 | `database/pgx` | [otelpgx](https://github.com/exaring/otelpgx) |
| database/sql | `database/sql` | [XSAM/otelsql](https://github.com/XSAM/otelsql) |
| sqlx | `database/sqlx` | [SQL adapter / XSAM/otelsql](https://github.com/XSAM/otelsql) |
| go-redis | `cache/redis` | [redisotel](https://github.com/redis/go-redis/tree/master/extra/redisotel) |
| NATS | `messaging/nats` | [Core and JetStream wrappers](https://github.com/nats-io/nats.go) |
| franz-go | `messaging/kafka/franzgo` | [kotel native hooks or compatibility wrappers](https://github.com/twmb/franz-go/tree/master/plugin/kotel) |
| kafka-go | `messaging/kafka/kafkago` | [native completion and reader wrappers](https://github.com/segmentio/kafka-go) |
| IBM Sarama | `messaging/kafka/sarama` | [native producer and consumer wrappers](https://github.com/IBM/sarama) |
| RabbitMQ | `messaging/rabbitmq` | [native channel/delivery wrappers](https://github.com/rabbitmq/amqp091-go) |

## PROVIDERS AND TELEMETRY

Pass your configured goslogx logger or another public `TelemetryProvider`. Plugins use its
tracer provider, meter provider and propagator, with explicit no-ops for absent signals.
The application owns provider flushing and shutdown. Plugins preserve native operation names,
span kinds, parentage, events, safe metadata and metric instruments where the upstream supports them.

Span metadata and metric dimensions follow separate policies. Keep request identifiers on
spans or logs when your privacy policy permits them; use bounded dimensions for metrics.
See the [integration policy](integration-policy.md) and individual integration guides for capture options.

## INTEGRATION DIFFERENCES

Bun query timing and pool observations use native instruments by default. Its compatibility
option can retain previous goslogx metrics during dashboard migration. pgx now uses otelpgx's
native hooks and `pgxpool.*` pool metrics. GORM's upstream metric path uses global providers,
so goslogx disables that path and measures public callbacks and SQL pool statistics through
the supplied provider.

go-pg's pgotel reference uses a package-global tracer without provider injection and requires
a recording parent. goslogx therefore uses its native QueryHook with explicit providers.
For Sarama, goslogx uses native channels and group callbacks; it does not overwrite application
Metadata or change delivery configuration to fit another adapter.

Use one instrumentation path for each operation. Do not combine direct upstream hooks,
goslogx hooks and automatic agents on the same client unless you have verified that their
signals do not overlap. Check the [plugin model](plugin-model.md) for ownership boundaries.
