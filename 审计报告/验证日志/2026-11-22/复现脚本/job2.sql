SET 'execution.checkpointing.interval' = '6s';
SET 'execution.checkpointing.mode' = 'EXACTLY_ONCE';
SET 'execution.checkpointing.min-pause' = '1s';
SET 'execution.checkpointing.timeout' = '5min';
SET 'parallelism.default' = '1';
SET 'pipeline.name' = 'eo-visibility-lag';
SET 'restart-strategy.type' = 'disable';

CREATE TABLE src (
  id INT
) WITH (
  'connector' = 'datagen',
  'number-of-rows' = '120',
  'rows-per-second' = '8',
  'fields.id.kind' = 'sequence',
  'fields.id.start' = '1',
  'fields.id.end' = '120'
);

CREATE TABLE kafka_sink (
  id INT
) WITH (
  'connector' = 'kafka',
  'topic' = 'eo-out2',
  'properties.bootstrap.servers' = 'kafka:9092',
  'sink.delivery-guarantee' = 'exactly-once',
  'sink.transactional-id-prefix' = 'flink-eo-vis',
  'sink.partitioner' = 'fixed',
  'properties.transaction.timeout.ms' = '900000',
  'format' = 'json'
);

INSERT INTO kafka_sink SELECT id FROM src;
