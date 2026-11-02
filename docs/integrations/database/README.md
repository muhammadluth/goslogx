# DATABASE INTEGRATIONS

| Library | Module | Implementation |
| --- | --- | --- |
| database/sql | `plugins/database/sql` | XSAM/otelsql |
| pgx | `plugins/database/pgx` | maintained otelpgx native hooks |
| Bun | `plugins/database/bun` | upstream bunotel with privacy facade |
| go-pg | `plugins/database/pg` | privacy-safe adaptation of upstream pgotel behavior |
| sqlx | `plugins/database/sqlx` | database/sql adapter |
| GORM | `plugins/database/gorm` | GORM OpenTelemetry plugin |
| Ent | `plugins/database/ent` | dialect driver wrapper |
| MongoDB | `plugins/database/mongo` | otelmongo monitors |
| ClickHouse | `plugins/database/clickhouse` | connection wrapper |
| Cassandra/ScyllaDB | `plugins/database/cassandra` | driver observer hooks |
| Elasticsearch | `plugins/database/elasticsearch` | upstream otelhttp transport with database scope |
| OpenSearch | `plugins/database/opensearch` | upstream otelhttp transport with database scope |
| Neo4j | `plugins/database/neo4j` | session wrapper |
| DynamoDB | `plugins/database/dynamodb` | otelaws AWS middleware |

Install the module matching the application's client. Statement capture is disabled by default or requires an explicit sanitizer where supported. Do not instrument both an ORM and its underlying driver unless the relevant guide documents two intentional abstraction layers.

Each document in this directory describes construction order, spans, metrics, errors, privacy, and lifecycle.

## DIAGNOSTIC PARITY

The plugins preserve each client's available operation semantics. Bun, go-pg,
pgx, database/sql, sqlx, and GORM can expose SQL details through explicit
sanitizer or raw diagnostic options. MongoDB uses command documents; Cassandra
uses CQL; Neo4j uses Cypher. Statement capture remains off by default.

`WithDBName` adds stable `db.namespace` and legacy `db.name` span metadata in
the supported adapters. Explicit `WithDBUser` and endpoint options are available
where documented; these values stay off metric dimensions. Caller capture is
opt-in only for integrations with a reliable call path. Pool metrics exist only
when the native client or driver exposes reliable statistics.

Elasticsearch and OpenSearch retain one HTTP CLIENT instrumentation layer for
transport requests. DynamoDB retains the AWS SDK operation spans. These clients
do not acquire SQL attributes or an extra database span for cosmetic parity.
