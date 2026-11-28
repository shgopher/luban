-- 44.11 锁实验：表 t 与原文一致（id 主键，c 普通索引，6 行数据）
CREATE DATABASE IF NOT EXISTS lab;
USE lab;
DROP TABLE IF EXISTS t;
CREATE TABLE t (
  id int NOT NULL,
  c  int DEFAULT NULL,
  d  int DEFAULT NULL,
  PRIMARY KEY (id),
  KEY c (c)
) ENGINE=InnoDB;
INSERT INTO t VALUES (0,0,0),(5,5,5),(10,10,10),(15,15,15),(20,20,20),(25,25,25);

-- 唯一键冲突实验用表
DROP TABLE IF EXISTS tu;
CREATE TABLE tu (
  id int NOT NULL AUTO_INCREMENT,
  c  int DEFAULT NULL,
  d  int DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY c (c)
) ENGINE=InnoDB;
INSERT INTO tu(c,d) VALUES (1,1),(2,2),(3,3),(4,4);
