-- MySQL Gap Lock Demo
-- Environment:
--   Engine: InnoDB
--   Isolation: Repeatable Read
-- Usage:
--   1. Run the setup section once.
--   2. Open two MySQL sessions: session A and session B.
--   3. Execute the statements in the marked order.

DROP TABLE IF EXISTS gap_lock_demo;

CREATE TABLE gap_lock_demo (
  id BIGINT NOT NULL AUTO_INCREMENT,
  age INT DEFAULT NULL,
  name VARCHAR(32) DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_age (age)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO gap_lock_demo (id, age, name) VALUES
  (1, 1, 'xiaoming'),
  (5, 5, 'xiaowang'),
  (7, 7, 'xiaozhang'),
  (11, 11, 'xiaochen');

-- =========================================================
-- Session defaults
-- Run in both sessions before each case.
-- =========================================================
-- SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ;

-- =========================================================
-- Case 1: unique index equality on an existing row
-- Expectation:
--   Session A locks row id=5.
--   Inserts id=3 / id=6 in session B should not be blocked by gap lock.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT * FROM gap_lock_demo WHERE id = 5 FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO gap_lock_demo VALUES (3, 3, 'case1_black'); -- usually not blocked
-- INSERT INTO gap_lock_demo VALUES (6, 6, 'case1_blue');  -- usually not blocked
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case 2: unique index equality on a missing row
-- Existing ids: 1,5,7,11
-- Query id=3, which falls in gap (1,5)
-- Expectation:
--   Insert id=2 should block.
--   Insert id=6 should not block.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT * FROM gap_lock_demo WHERE id = 3 FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO gap_lock_demo VALUES (6, 6, 'case2_blue');   -- usually not blocked
-- INSERT INTO gap_lock_demo VALUES (2, 2, 'case2_yellow'); -- blocked
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case 3: unique index range query
-- Expectation:
--   Query scans from id=5 toward the first non-matching value.
--   Insert id=6 or id=7-adjacent values may be blocked depending on range end.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT * FROM gap_lock_demo WHERE id >= 5 AND id < 6 FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO gap_lock_demo VALUES (6, 6, 'case3_green'); -- often blocked
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case 4: non-unique index range query
-- Expectation:
--   Lock range is usually larger than a single matching point on idx_age.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT * FROM gap_lock_demo WHERE age >= 5 AND age < 6 FOR UPDATE;

-- Session B
-- BEGIN;
-- INSERT INTO gap_lock_demo VALUES (2, 2, 'case4_yellow'); -- may block due to idx_age gap
-- INSERT INTO gap_lock_demo VALUES (8, 8, 'case4_green');  -- often not blocked
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Case 5: deadlock around the same gap
-- Expectation:
--   Both sessions first lock the same logical gap.
--   Then both try to insert into that gap and may deadlock.
-- =========================================================
-- Session A
-- BEGIN;
-- SELECT * FROM gap_lock_demo WHERE id = 3 FOR UPDATE;

-- Session B
-- BEGIN;
-- SELECT * FROM gap_lock_demo WHERE id = 4 FOR UPDATE;

-- Session A
-- INSERT INTO gap_lock_demo VALUES (2, 2, 'case5_yellow');

-- Session B
-- INSERT INTO gap_lock_demo VALUES (4, 4, 'case5_purple');

-- One side should hit:
-- ERROR 1213 (40001): Deadlock found when trying to get lock

-- =========================================================
-- Case 6: limit changes the scan stop position
-- To make this case easier to observe, add age=6 first.
-- =========================================================
-- INSERT INTO gap_lock_demo VALUES (6, 6, 'case6_seed');

-- Session A
-- BEGIN;
-- DELETE FROM gap_lock_demo WHERE age = 6 LIMIT 1;

-- Session B
-- BEGIN;
-- INSERT INTO gap_lock_demo VALUES (6, 6, 'case6_reinsert');
-- ROLLBACK;

-- Session A
-- ROLLBACK;

-- =========================================================
-- Mapping to content_order
-- Think about these two queries under RR:
--
-- 1) Narrow lock by unique index
-- SELECT * FROM content_order
-- WHERE order_id = 'CO_123' AND order_type = 1
-- FOR UPDATE;
--
-- 2) Wider lock by secondary index range
-- SELECT order_id FROM content_order
-- WHERE order_type = 1 AND user_id = 10001 AND status = 0
-- FOR UPDATE;
--
-- The first one behaves closer to record-lock style access.
-- The second one is much more likely to create a locked index range.
