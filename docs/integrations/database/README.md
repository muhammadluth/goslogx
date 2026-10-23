# DATABASE INTEGRATIONS

| Library | Module | Implementation |
| --- | --- | --- |
| database/sql | `plugins/database/sql` | XSAM/otelsql |
| pgx | `plugins/database/pgx` | pgx tracer hooks |
| Bun | `plugins/database/bun` | Bun QueryHook |
| go-pg | `plugins/database/pg` | go-pg QueryHook |
| sqlx | `plugins/database/sqlx` | database/sql adapter |
| GORM | `plugins/database/gorm` | GORM OpenTelemetry plugin |
| Ent | `plugins/database/ent` | dialect driver wrapper |
| MongoDB | `plugins/database/mongo` | otelmongo monitors |
| ClickHouse | `plugins/database/clickhouse` | connection wrapper |
| Cassandra/ScyllaDB | `plugins/database/cassandra` | driver observer hooks |
| Elasticsearch | `plugins/database/elasticsearch` | instrumented HTTP transport |
| OpenSearch | `plugins/database/opensearch` | instrumented HTTP transport |
| Neo4j | `plugins/database/neo4j` | session wrapper |
| DynamoDB | `plugins/database/dynamodb` | otelaws AWS middleware |

Install the module matching the application's client. Statement capture is disabled by default or requires an explicit sanitizer where supported. Do not instrument both an ORM and its underlying driver unless the relevant guide documents two intentional abstraction layers.

Each document in this directory describes construction order, spans, metrics, errors, privacy, and lifecycle.
