SET 'execution.checkpointing.interval' = '3s';
SET 'execution.checkpointing.mode' = 'EXACTLY_ONCE';
SET 'execution.checkpointing.min-pause' = '1s';
SET 'execution.checkpointing.timeout' = '5min';
SET 'parallelism.default' = '1';
SET 'pipeline.name' = 'eo-kafka-sink-test';
SET 'restart-strategy.type' = 'disable';

CREATE TABLE src (
  id INT
) WITH (
  'connector' = 'datagen',
  'number-of-rows' = '400',
  'rows-per-second' = '20',
  'fields.id.kind' = 'sequence',
  'fields.id.start' = '1',
  'fields.id.end' = '400'
);

CREATE TABLE kafka_sink (
  id INT
) WITH (
  'connector' = 'kafka',
  'topic' = 'eo-out',
  'properties.bootstrap.servers' = 'kafka:9092',
  'sink.delivery-guarantee' = 'exactly-once',
  'sink.transactional-id-prefix' = 'flink-eo-test',
  'sink.partitioner' = 'fixed',
  'properties.transaction.timeout.ms' = '900000',
  'format' = 'json'
);

INSERT INTO kafka_sink SELECT id FROM src;
