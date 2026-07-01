// dbdeduct 演示:在 Go 项目里如何实现腾讯云"热点更新"那套语法想达到的效果。
//
// 关键认知:那几个关键字(COMMIT_ON_SUCCESS / ROLLBACK_ON_FAIL /
// QUEUE_ON_PK / TARGET_AFFECT_ROW)是腾讯云 Percona 定制内核的私有语法,
// 标准 MySQL 不认识。Go 的 database/sql 不解析 SQL 方言,所以:
//   - 库支持那套语法 → 直接把 SQL 当普通字符串 Exec(见 hotUpdateRaw)。
//   - 标准 MySQL → 用"单条原子条件更新 + 看 RowsAffected"达成同样目的
//     (见 deductStandard),这才是你真正要写的。
//
// 本文件仅用标准库 database/sql 即可编译。要真正连库运行,需:
//   go get github.com/go-sql-driver/mysql
// 然后在本文件加一行空导入: _ "github.com/go-sql-driver/mysql"
// 并设置环境变量 STOCK_DSN(如 user:pass@tcp(127.0.0.1:3306)/db)。
//
// 运行(无 DSN 时只打印说明与 SQL): go run ./hotrow/dbdeduct
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

var ErrSoldOut = errors.New("库存不足/售罄")

// ───────────────────────────────────────────────────────────────────
// A. 腾讯云私有语法:Go 里没有特殊写法,就是一个普通字符串。
//    仅当你的 DB 是那套定制内核才有效;标准 MySQL 会直接报语法错误。
// ───────────────────────────────────────────────────────────────────
func hotUpdateRaw(ctx context.Context, db *sql.DB, id int64) error {
	const q = `UPDATE stock SET num = num - 1 WHERE id = ?
	           COMMIT_ON_SUCCESS ROLLBACK_ON_FAIL TARGET_AFFECT_ROW 1`
	_, err := db.ExecContext(ctx, q, id) // database/sql 不关心方言,原样发给 DB
	return err
}

// ───────────────────────────────────────────────────────────────────
// B. 标准 MySQL 的等价实现(推荐):单条原子条件更新 + 看 RowsAffected。
//    这一条语句在 autocommit 下:取行锁 → 改 → 提交 → 放锁,一次往返完成,
//    锁占用时间 = 这条语句本身(中间没有任何应用逻辑),正是热点更新的目的。
//
//    关键字逐一映射:
//      COMMIT_ON_SUCCESS   = autocommit 单语句,成功即自动提交
//      ROLLBACK_ON_FAIL    = WHERE num>=? 不命中则改 0 行、无副作用(天然"回滚")
//      TARGET_AFFECT_ROW 1 = Go 里判断 RowsAffected()==1
//      QUEUE_ON_PK id      = WHERE id=?(主键),InnoDB 行锁天然在该行上排队
// ───────────────────────────────────────────────────────────────────
func deductStandard(ctx context.Context, db *sql.DB, id, n int64) error {
	res, err := db.ExecContext(ctx,
		`UPDATE stock SET num = num - ? WHERE id = ? AND num >= ?`, n, id, n)
	if err != nil {
		return err // 真正的执行错误(网络/死锁等),需重试或上报
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil // 扣减成功(已提交)
	}
	return ErrSoldOut // 改了 0 行 = 库存不足,无副作用,无需显式回滚
}

// ───────────────────────────────────────────────────────────────────
// C. 需要"扣库存 + 建订单"一起原子时:放进一个事务。
//    注意陷阱:行锁会从 UPDATE 一直持有到 COMMIT,期间的建单逻辑都在持锁中,
//    又退化成"长时间持锁"。所以务必:把扣库存放到事务最后一步、事务体尽量小。
//    这正是腾讯云那个私有特性存在的理由——避免跨应用逻辑持有热点行锁。
// ───────────────────────────────────────────────────────────────────
func deductInTx(ctx context.Context, db *sql.DB, id int64, orderSQL string) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback() // ROLLBACK_ON_FAIL:任何一步失败全回滚
		}
	}()

	// 建议:先做别的(建单等),把扣库存放最后,最小化持锁窗口
	if _, err = tx.ExecContext(ctx, orderSQL); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE stock SET num = num - 1 WHERE id = ? AND num >= 1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrSoldOut // 触发上面的 Rollback
	}
	return tx.Commit() // COMMIT_ON_SUCCESS
}

func main() {
	ctx := context.Background()
	dsn := os.Getenv("STOCK_DSN")

	if dsn == "" {
		fmt.Println("未设置 STOCK_DSN,仅打印示例 SQL(不连库)。")
		fmt.Println("要真正运行: go get github.com/go-sql-driver/mysql,")
		fmt.Println("在本文件加 _ \"github.com/go-sql-driver/mysql\",再设 STOCK_DSN。\n")
		fmt.Println("标准 MySQL 推荐写法(原子扣减):")
		fmt.Println("  UPDATE stock SET num = num - 1 WHERE id = ? AND num >= 1;")
		fmt.Println("  -- 然后在 Go 里:RowsAffected()==1 → 成功;==0 → 库存不足")
		fmt.Println("\n建表参考:")
		fmt.Println("  CREATE TABLE stock(id BIGINT PRIMARY KEY, num INT NOT NULL);")
		return
	}

	db, err := sql.Open("mysql", dsn) // 需已空导入 mysql 驱动,否则这里报 unknown driver
	if err != nil {
		fmt.Println("打开数据库失败:", err)
		return
	}
	defer db.Close()

	switch err := deductStandard(ctx, db, 1, 1); {
	case err == nil:
		fmt.Println("扣减成功")
	case errors.Is(err, ErrSoldOut):
		fmt.Println("售罄")
	default:
		fmt.Println("出错:", err)
	}
}
