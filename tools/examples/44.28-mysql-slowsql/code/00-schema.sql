-- 44.28 MySQL 慢 SQL 案例集：建表 + 灌数
-- 环境：MySQL 8.0.46，RR，innodb_page_size=16384，buffer pool 128M
-- 每张表 5 万行：够让优化器在「索引 vs 全表」之间真的纠结，又不至于灌数占太久

CREATE DATABASE IF NOT EXISTS lab;
USE lab;

SET SESSION cte_max_recursion_depth = 100000;

DROP TABLE IF EXISTS tradelog, idcard, words, tcity;

-- ---------- 案例 1/2：隐式类型转换、函数操作 ----------
-- tradeid 是 varchar(32)，值形如 '110001'..'160000'，能当数字用，正好制造隐式转换
-- t_modified 从 2016-01-01 起每行加 31 分钟，铺满 2016~2018 三年
CREATE TABLE tradelog (
  id          INT NOT NULL AUTO_INCREMENT,
  tradeid     VARCHAR(32)  NOT NULL,
  operator    INT          NOT NULL,
  t_modified  DATETIME     NOT NULL,
  amount      INT          NOT NULL,
  PRIMARY KEY (id),
  KEY idx_tradeid (tradeid),
  KEY idx_tmodified (t_modified)
) ENGINE=InnoDB;

INSERT INTO tradelog (tradeid, operator, t_modified, amount)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 50000
)
SELECT CAST(n + 110000 AS CHAR),
       n % 50,
       TIMESTAMPADD(MINUTE, n * 31, CAST('2016-01-01 00:00:00' AS DATETIME)),
       n % 1000
FROM seq;

-- ---------- 案例 3：前缀索引的区分度 ----------
-- 身份证号 = 6 位地址码（只有 20 个县）+ 8 位生日 + 4 位顺序号
-- 前 6 位重复度极高，正是原文「前缀区分度不够」的场景
CREATE TABLE idcard (
  id       INT NOT NULL AUTO_INCREMENT,
  id_card  VARCHAR(18) NOT NULL,
  name     VARCHAR(16) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_card (id_card)
) ENGINE=InnoDB;

INSERT INTO idcard (id_card, name)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 50000
)
SELECT CONCAT(
         LPAD(330100 + (n % 20) * 7, 6, '0'),
         DATE_FORMAT(TIMESTAMPADD(DAY, n % 730, CAST('1970-01-01' AS DATETIME)), '%Y%m%d'),
         LPAD(n DIV 20, 4, '0')
       ),
       CONCAT('user', n)
FROM seq;

-- ---------- 案例 4/5：count 与 order by ----------
-- city 只有 5 个值，杭州占 30%（15000 行），和原文 4000 行的市民表同一量级
CREATE TABLE tcity (
  id    INT NOT NULL,
  city  VARCHAR(16) NOT NULL,
  name  VARCHAR(16) NOT NULL,
  age   INT NOT NULL,
  addr  VARCHAR(128) DEFAULT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB;

INSERT INTO tcity (id, city, name, age, addr)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 50000
)
SELECT n,
       ELT(n % 5 + 1, '杭州', '苏州', '北京', '上海', '广州'),
       CONCAT(CHAR(65 + n % 26), LPAD(n, 6, '0')),
       18 + n % 50,
       CONCAT('浙江省杭州市西湖区文一西路 ', n, ' 号')
FROM seq;

-- ---------- 案例 6：随机消息 ----------
CREATE TABLE words (
  id   INT NOT NULL AUTO_INCREMENT,
  word VARCHAR(64) DEFAULT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB;

INSERT INTO words (word)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 50000
)
SELECT CONCAT(CHAR(97 + (n DIV 1000) % 26), CHAR(97 + (n DIV 100) % 26), LPAD(n, 5, '0'))
FROM seq;

ANALYZE TABLE tradelog, idcard, tcity, words;

SELECT 'tradelog' AS t, COUNT(*) AS rows_ FROM tradelog
UNION ALL SELECT 'idcard', COUNT(*) FROM idcard
UNION ALL SELECT 'tcity',  COUNT(*) FROM tcity
UNION ALL SELECT 'words',  COUNT(*) FROM words;
