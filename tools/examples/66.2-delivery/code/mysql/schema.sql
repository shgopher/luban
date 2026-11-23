-- 66.2 消息可靠投递：MySQL 8.0+ 消息表 / 收件箱 / 回执表设计稿（本机 MySQL 26.7.0 实测）
CREATE TABLE message (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  conv_id       BIGINT UNSIGNED NOT NULL,
  conv_seq      BIGINT UNSIGNED NOT NULL,          -- 会话内单调序号：排序与续传的唯一依据
  msg_id        VARCHAR(40)     NOT NULL,          -- 服务端全局 ID（发号器生成，见 44.24）
  sender_id     BIGINT UNSIGNED NOT NULL,
  client_msg_id VARCHAR(64)     NOT NULL,          -- 客户端生成，重试复用；上行幂等键
  body          VARCHAR(2048)   NOT NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  status        TINYINT         NOT NULL DEFAULT 0 COMMENT '0=STORED 1=DELIVERED 2=READ',
  PRIMARY KEY (id),
  UNIQUE KEY uk_conv_seq    (conv_id, conv_seq),   -- 同一会话内 seq 绝不重复
  UNIQUE KEY uk_sender_cmid (sender_id, client_msg_id), -- 上行幂等：重复发送不会新建行
  KEY idx_conv_created (conv_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 设备级送达回执（可批量合并），主键天然幂等
CREATE TABLE delivery_ack (
  user_id     BIGINT UNSIGNED NOT NULL,
  device_id   BIGINT UNSIGNED NOT NULL,
  conv_id     BIGINT UNSIGNED NOT NULL,
  ack_seq     BIGINT UNSIGNED NOT NULL,  -- 已连续送达的最大 seq（不是最后收到的那条）
  updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id, device_id, conv_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 离线收件箱（按人存，参考实现；线上热路径用 Redis ZSET，冷数据落这里）
CREATE TABLE offline_inbox (
  user_id    BIGINT UNSIGNED NOT NULL,
  conv_seq   BIGINT UNSIGNED NOT NULL,  -- 用户维度单调序号（多端同步用，见 66.3）
  msg_id     VARCHAR(40)     NOT NULL,
  conv_id    BIGINT UNSIGNED NOT NULL,
  expire_at  DATETIME        NOT NULL,  -- 保留策略：时间上限
  PRIMARY KEY (user_id, conv_seq),
  KEY idx_expire (expire_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
