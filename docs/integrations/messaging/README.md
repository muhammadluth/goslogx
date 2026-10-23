# MESSAGING INTEGRATIONS

goslogx supports [NATS](nats.md), [Kafka](kafka.md), and [RabbitMQ](rabbitmq.md) through optional modules. Messaging plugins inject trace context into outgoing headers and extract it before consumer processing.

Applications retain ownership of delivery, acknowledgement, retry, dead-letter, and broker shutdown behavior. Pass the extracted context into processing functions and context-aware logs. Payloads and raw message keys are not telemetry attributes by default.

Normalize destination names when subjects, topics, queues, or routing keys contain tenant or entity identifiers. Avoid enabling another instrumentation layer for the same client because it can create duplicate producer and consumer spans.
