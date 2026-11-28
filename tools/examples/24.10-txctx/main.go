// 24.10 的事务对象演示：用内存表模拟数据库，跑通「tx 沿 ctx 传递」的四种命运。
//
// 只依赖标准库，`go run .` 直接跑。模拟的是「事务对象怎么传」这个结构问题，
// 不是 MySQL 的隔离级别；GORM 的 Commit/Rollback 语义在这里被简化成
// 「把暂存行追加进已提交列表」。
package main

import (
	"context"
	"errors"
	"fmt"
)

// row 是一行待写入的数据；真实系统里是整行记录，这里只留定位用的键。
type row struct {
	table string
	key   string
}

// DB 是最小的“数据库”：committed 相当于已落盘的数据。
// activeTx 用来暴露一个真实故障：panic 之后既没提交也没回滚，连接不归还。
type DB struct {
	committed []row
	activeTx  int
}

// Tx 是一个未决事务。staged 是它自己的脏页，提交前别人看不见。
type Tx struct {
	db     *DB
	staged []row
	done   bool
}

func (d *DB) Begin() *Tx {
	d.activeTx++
	return &Tx{db: d}
}

func (t *Tx) Insert(table, key string) {
	t.staged = append(t.staged, row{table, key})
}

func (t *Tx) Commit() {
	if t.done {
		return
	}
	t.done = true
	t.db.committed = append(t.db.committed, t.staged...)
	t.db.activeTx--
}

func (t *Tx) Rollback() {
	if t.done {
		return
	}
	t.done = true
	t.db.activeTx--
}

// txKey 是 ctx 里放事务的键；用私有空结构体，避免与别的包撞键。
type txKey struct{}

// Store 相当于仓储层：业务方法签名里没有 tx，它只从 ctx 里问。
type Store struct{ db *DB }

// TX 是事务边界：开事务、把 tx 放进 ctx、按 fn 的返回值决定提交或回滚。
func (s *Store) TX(ctx context.Context, fn func(context.Context) error) (err error) {
	// 嵌套调用：GORM 用保存点实现，内层失败只回滚到保存点，外层还能继续。
	if parent, ok := ctx.Value(txKey{}).(*Tx); ok {
		return s.savepoint(ctx, parent, fn)
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback() // 少了这行：panic 后事务悬挂，连接不归还
			panic(r)
		}
		if err != nil {
			tx.Rollback()
			return
		}
		tx.Commit()
	}()
	return fn(context.WithValue(ctx, txKey{}, tx))
}

// TXNoDefer 是反面写法：panic 时不回滚也不提交。
func (s *Store) TXNoDefer(ctx context.Context, fn func()) {
	tx := s.db.Begin()
	_ = context.WithValue(ctx, txKey{}, tx) // 故意丢掉返回值：ctx 没传下去
	fn()
	tx.Commit()
}

// savepoint 模拟 GORM 的嵌套事务：记住当前长度，失败就截断回去。
func (s *Store) savepoint(ctx context.Context, tx *Tx, fn func(context.Context) error) error {
	mark := len(tx.staged)
	if err := fn(ctx); err != nil {
		tx.staged = tx.staged[:mark] // 回滚到保存点，外层不受影响
		return err
	}
	return nil
}

// Exec 是业务真正调用的写方法：ctx 里有事务就用事务，没有就直接落库。
func (s *Store) Exec(ctx context.Context, table, key string) {
	if tx, ok := ctx.Value(txKey{}).(*Tx); ok {
		tx.Insert(table, key)
		return
	}
	s.db.committed = append(s.db.committed, row{table, key}) // 没有事务：立即生效
}

// fmtRows 把提交结果压成 "table:key" 列表，方便比对。
func fmtRows(rows []row) string {
	if len(rows) == 0 {
		return "[]"
	}
	out := "["
	for i, r := range rows {
		if i > 0 {
			out += " "
		}
		out += r.table + ":" + r.key
	}
	return out + "]"
}

func main() {
	fmt.Println("① 正确：事务内两次写，第二次失败 → 全部回滚")
	{
		db := &DB{}
		s := &Store{db: db}
		_ = s.TX(context.Background(), func(ctx context.Context) error {
			s.Exec(ctx, "uc_user", "u1")
			s.Exec(ctx, "uc_secret", "s1")
			return errors.New("secret 写失败")
		})
		fmt.Printf("   已提交：%s  activeTx=%d\n", fmtRows(db.committed), db.activeTx)
	}

	fmt.Println("② 子函数换成 Background → 事务丢失，留下孤儿行")
	{
		db := &DB{}
		s := &Store{db: db}
		_ = s.TX(context.Background(), func(ctx context.Context) error {
			s.Exec(ctx, "uc_user", "u2")
			// 典型写法：起 goroutine / 调子函数时顺手用了 Background
			sub(context.Background(), s)
			return errors.New("secret 写失败")
		})
		fmt.Printf("   已提交：%s  activeTx=%d\n", fmtRows(db.committed), db.activeTx)
	}

	fmt.Println("③ panic 且没有 defer Rollback → 事务悬挂")
	{
		db := &DB{}
		s := &Store{db: db}
		func() {
			defer func() { _ = recover() }()
			s.TXNoDefer(context.Background(), func() { panic("boom") })
		}()
		fmt.Printf("   已提交：%s  activeTx=%d\n", fmtRows(db.committed), db.activeTx)
	}

	fmt.Println("④ 嵌套事务 = 保存点：内层回滚，外层提交")
	{
		db := &DB{}
		s := &Store{db: db}
		_ = s.TX(context.Background(), func(ctx context.Context) error {
			s.Exec(ctx, "uc_user", "u3")
			_ = s.TX(ctx, func(ctx context.Context) error {
				s.Exec(ctx, "uc_secret", "s3")
				return errors.New("secret 写失败")
			})
			return nil // 外层吞掉内层错误，继续提交
		})
		fmt.Printf("   已提交：%s  activeTx=%d\n", fmtRows(db.committed), db.activeTx)
	}
}

// sub 是“事务内调用的子函数”，演示 ctx 传丢的后果。
func sub(ctx context.Context, s *Store) {
	s.Exec(ctx, "uc_secret", "s2")
}
