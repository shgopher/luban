// e1_bitmap: 100 万用户对同一条内容的点赞状态，bitmap(SETBIT) vs SET(SADD) 的真实内存与写入耗时。
// 每个方案前后各读一次 INFO memory，差值即该方案实际占用；再单独读 MEMORY USAGE / DEBUG OBJECT。
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	nUsers  = 1_000_000
	objID   = 1
	pipeLen = 1000
)

var ctx = context.Background()

func usedMemory(rdb *redis.Client) int64 {
	info, err := rdb.Info(ctx, "memory").Result()
	if err != nil {
		panic(err)
	}
	// 只取 used_memory: 这一行
	start := 0
	for i := 0; i < len(info); i++ {
		if i+12 <= len(info) && info[i:i+12] == "used_memory:" {
			start = i + 12
			break
		}
	}
	end := start
	for end < len(info) && info[end] != '\r' && info[end] != '\n' {
		end++
	}
	v, _ := strconv.ParseInt(info[start:end], 10, 64)
	return v
}

func memUsage(rdb *redis.Client, key string) int64 {
	v, err := rdb.MemoryUsage(ctx, key).Result()
	if err != nil {
		return -1
	}
	return v
}

func debugObj(rdb *redis.Client, key string) string {
	v, err := rdb.Do(ctx, "DEBUG", "OBJECT", key).Text()
	if err != nil {
		return "ERR " + err.Error()
	}
	return v
}

func flush(rdb *redis.Client) {
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		panic(err)
	}
}

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:16394",
		PoolSize:     8,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Println("redis ping failed:", err)
		os.Exit(1)
	}

	bitmapKey := fmt.Sprintf("like:bitmap:obj:%d", objID)
	setKey := fmt.Sprintf("like:set:obj:%d", objID)

	// ---------- 方案 A：bitmap，bit 偏移 = user_id ----------
	flush(rdb)
	baseA := usedMemory(rdb)
	t0 := time.Now()
	pipe := rdb.Pipeline()
	n := 0
	for u := 0; u < nUsers; u++ {
		pipe.SetBit(ctx, bitmapKey, int64(u), 1)
		n++
		if n%pipeLen == 0 {
			if _, err := pipe.Exec(ctx); err != nil {
				fmt.Println("SETBIT pipeline error:", err)
				os.Exit(1)
			}
			pipe = rdb.Pipeline()
		}
	}
	if n%pipeLen != 0 {
		if _, err := pipe.Exec(ctx); err != nil {
			fmt.Println("SETBIT pipeline tail error:", err)
			os.Exit(1)
		}
	}
	durA := time.Since(t0)
	afterA := usedMemory(rdb)
	bitmapStrlen, _ := rdb.StrLen(ctx, bitmapKey).Result()
	bitmapBits, _ := rdb.BitCount(ctx, bitmapKey, nil).Result()
	bitmapMem := memUsage(rdb, bitmapKey)

	fmt.Println("== 方案 A: bitmap (SETBIT x1,000,000, bit offset = dense uid 0..999999) ==")
	fmt.Printf("elapsed                     : %v\n", durA.Round(time.Millisecond))
	fmt.Printf("SETBIT commands             : %d\n", nUsers)
	fmt.Printf("STRLEN (bytes)              : %d\n", bitmapStrlen)
	fmt.Printf("BITCOUNT                    : %d\n", bitmapBits)
	fmt.Printf("MEMORY USAGE key            : %d bytes\n", memUsage(rdb, bitmapKey))
	fmt.Printf("INFO used_memory delta      : %d bytes\n", afterA-baseA)
	fmt.Printf("INFO used_memory_before/after: %d / %d\n", baseA, afterA)
	fmt.Printf("DEBUG OBJECT                : %s\n", debugObj(rdb, bitmapKey))

	// ---------- 方案 B：SET，member = user_id ----------
	flush(rdb)
	baseB := usedMemory(rdb)
	t0 = time.Now()
	pipe = rdb.Pipeline()
	n = 0
	for u := 0; u < nUsers; u++ {
		pipe.SAdd(ctx, setKey, u)
		n++
		if n%pipeLen == 0 {
			if _, err := pipe.Exec(ctx); err != nil {
				fmt.Println("SADD pipeline error:", err)
				os.Exit(1)
			}
			pipe = rdb.Pipeline()
		}
	}
	if n%pipeLen != 0 {
		if _, err := pipe.Exec(ctx); err != nil {
			fmt.Println("SADD pipeline tail error:", err)
			os.Exit(1)
		}
	}
	durB := time.Since(t0)
	afterB := usedMemory(rdb)
	setCard, _ := rdb.SCard(ctx, setKey).Result()
	setMem := memUsage(rdb, setKey)

	fmt.Println()
	fmt.Println("== 方案 B: SET (SADD x1,000,000, member = dense uid 0..999999) ==")
	fmt.Printf("elapsed                     : %v\n", durB.Round(time.Millisecond))
	fmt.Printf("SADD commands               : %d\n", nUsers)
	fmt.Printf("SCARD                       : %d\n", setCard)
	fmt.Printf("MEMORY USAGE key            : %d bytes\n", setMem)
	fmt.Printf("INFO used_memory delta      : %d bytes\n", afterB-baseB)
	fmt.Printf("INFO used_memory_before/after: %d / %d\n", baseB, afterB)
	fmt.Printf("DEBUG OBJECT                : %s\n", debugObj(rdb, setKey))

	// ---------- 读数 ----------
	fmt.Println()
	fmt.Println("== 读数（本机实测） ==")
	fmt.Printf("bitmap 每用户            : %.4f B  (12.50 MB/key for 100M users = %.2f MiB)\n",
		float64(bitmapStrlen)/nUsers, float64(100_000_000)/8/1024/1024)
	fmt.Printf("SET    每用户            : %.2f B  (MEMORY USAGE / 1,000,000)\n", float64(setMem)/nUsers)
	fmt.Printf("SET/bitmap 内存倍数       : %.2f x（对有效位 STRLEN）/ %.2f x（对 MEMORY USAGE）\n",
		float64(setMem)/float64(bitmapStrlen), float64(setMem)/float64(bitmapMem))
	fmt.Printf("推算 1 亿用户 bitmap      : 12.50 MB（12,500,000 B，%.2f MiB）\n", float64(100_000_000)/8/1024/1024)
	fmt.Printf("推算 1 亿用户 SET         : %.2f GB（%.0f B，%.2f GiB）\n",
		float64(setMem)/nUsers*100_000_000/1e9, float64(setMem)/nUsers*100_000_000, float64(setMem)/nUsers*100_000_000/1024/1024/1024)

	// 清理，避免影响后续实验与并行任务（自己的实例）
	flush(rdb)
}
