-- content_order-oriented gap lock demo
-- Goal:
--   Contrast narrow locking on a unique key with wider locking on a secondary index.
--
-- Environment:
--   MySQL / InnoDB / REPEATABLE READ
--   Use two sessions: A and B

DROP TABLE IF EXISTS content_order_gap_demo;

CREATE TABLE content_order_gap_demo (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  order_id VARCHAR(64) NOT NULL DEFAULT '',
  order_type INT NOT NULL,
  user_id BIGINT NOT NULL,
  status INT NOT NULL,
  item_id VARCHAR(64) NOT NULL DEFAULT '',
  item_sku VARCHAR(64) NOT NULL DEFAULT '',
  create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uniq_order (order_id, order_type),
  KEY idx_user_orders (order_type, user_id, status, create_time),
  KEY idx_user_items (order_type, user_id, item_id, item_sku, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO content_order_gap_demo
  (order_id, order_type, user_id, status, item_id, item_sku, create_time)
VALUES
  ('CO_1001', 1, 9001, 0, 'ITEM_A', 'SKU_1', '2026-04-13 10:00:00'),
  ('CO_1005', 1, 9001, 0, 'ITEM_A', 'SKU_1', '2026-04-13 10:05:00'),
  ('CO_1007', 1, 9001, 1, 'ITEM_A', 'SKU_1', '2026-04-13 10:07:00'),
  ('CO_1011', 1, 9002, 0, 'ITEM_B', 'SKU_2', '2026-04-13 10:11:00');

-- Run before each case in both sessions:
-- SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ;

-- =========================================================
-- Case A: precise locking via unique key
-- Expectation:
--   Session A locks one existing order by uniq_order(order_id, order_type).
--   Session B can still insert other order_ids freely.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT * FROM content_order_gap_demo
-- WHERE order_id = 'CO_1005' AND order_type = 1
-- FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO content_order_gap_demo
--   (order_id, order_type, user_id, status, item_id, item_sku, create_time)
-- VALUES
--   ('CO_1006', 1, 9001, 0, 'ITEM_A', 'SKU_1', NOW()); -- usually not blocked
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case B: probing a missing unique key can still block nearby inserts
-- Existing unique keys around probe:
--   CO_1001, CO_1005, CO_1007, CO_1011
-- Querying a missing key in between may lock the adjacent unique-key gap.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT * FROM content_order_gap_demo
-- WHERE order_id = 'CO_1003' AND order_type = 1
-- FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO content_order_gap_demo
--   (order_id, order_type, user_id, status, item_id, item_sku, create_time)
-- VALUES
--   ('CO_1002', 1, 9001, 0, 'ITEM_A', 'SKU_1', NOW()); -- may block
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case C: wider lock on idx_user_orders
-- Expectation:
--   Session A scans a secondary-index range for one user's pending orders.
--   Session B inserts another pending order for the same user/range and may block.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT order_id, create_time
-- FROM content_order_gap_demo
-- WHERE order_type = 1
--   AND user_id = 9001
--   AND status = 0
-- FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO content_order_gap_demo
--   (order_id, order_type, user_id, status, item_id, item_sku, create_time)
-- VALUES
--   ('CO_1004', 1, 9001, 0, 'ITEM_A', 'SKU_1', '2026-04-13 10:04:00'); -- may block
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case D: same user, different status may avoid the locked range
-- Expectation:
--   If status is part of the same secondary index path,
--   inserting status=1 may behave differently from status=0.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT order_id
-- FROM content_order_gap_demo
-- WHERE order_type = 1
--   AND user_id = 9001
--   AND status = 0
-- FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO content_order_gap_demo
--   (order_id, order_type, user_id, status, item_id, item_sku, create_time)
-- VALUES
--   ('CO_1008', 1, 9001, 1, 'ITEM_A', 'SKU_1', NOW()); -- often less affected
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case E: LIMIT changes the stop position on secondary index scanning
-- Expectation:
--   LIMIT 1 may reduce how far the engine scans on idx_user_orders.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT order_id
-- FROM content_order_gap_demo
-- WHERE order_type = 1
--   AND user_id = 9001
--   AND status = 0
-- ORDER BY create_time
-- LIMIT 1
-- FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO content_order_gap_demo
--   (order_id, order_type, user_id, status, item_id, item_sku, create_time)
-- VALUES
--   ('CO_1003B', 1, 9001, 0, 'ITEM_A', 'SKU_1', '2026-04-13 10:03:00');
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- Suggested verification:
-- EXPLAIN SELECT * FROM content_order_gap_demo
-- WHERE order_id = 'CO_1005' AND order_type = 1 FOR UPDATE;
--
-- EXPLAIN SELECT order_id FROM content_order_gap_demo
-- WHERE order_type = 1 AND user_id = 9001 AND status = 0 FOR UPDATE;
