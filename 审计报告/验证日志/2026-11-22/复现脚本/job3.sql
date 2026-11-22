SET 'execution.checkpointing.interval' = '5s';
SET 'parallelism.default' = '1';
SET 'pipeline.name' = 'gmv-tumble';

CREATE TABLE orders (
  order_id STRING,
  region   STRING,
  amount   DECIMAL(18,2),
  ts       TIMESTAMP(3),
  WATERMARK FOR ts AS ts - INTERVAL '5' SECOND
) WITH (
  'connector' = 'kafka',
  'topic' = 'orders',
  'properties.bootstrap.servers' = 'kafka:9092',
  'properties.group.id' = 'gmv-v1',
  'scan.startup.mode' = 'earliest-offset',
  'scan.bounded.mode' = 'latest-offset',
  'format' = 'json'
);

CREATE TABLE gmv_sink (
  window_start TIMESTAMP(3),
  window_end   TIMESTAMP(3),
  region       STRING,
  gmv          DECIMAL(18,2),
  PRIMARY KEY (window_start, region) NOT ENFORCED
) WITH (
  'connector' = 'upsert-kafka',
  'topic' = 'gmv',
  'properties.bootstrap.servers' = 'kafka:9092',
  'key.format' = 'json',
  'key.fields' = 'window_start,region',
  'value.format' = 'json'
);

INSERT INTO gmv_sink
SELECT window_start, window_end, region, SUM(amount)
FROM TABLE(TUMBLE(TABLE orders, DESCRIPTOR(ts), INTERVAL '1' MINUTE))
GROUP BY window_start, window_end, region;
