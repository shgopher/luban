package main

import "testing"

func base() input {
	return input{
		readQPS: 60000, writeTPS: 800, readPerNode: 12000, cacheAbsorb: 0.3,
		staleBudget: 500, rpoSec: 0, rtoSec: 60, rows: 5e7, dataGB: 800,
		az: 3, pricePerNode: 2000, binlogMBps: 2,
	}
}

func TestReplicaCountCoversAZRedundancy(t *testing.T) {
	in := base()
	p := compute(in)
	// 60000×0.7=42000 有效读；×1.5 AZ 冗余=63000；÷12000 → 6 个副本
	if p.replicas != 6 {
		t.Fatalf("want 6 replicas, got %d", p.replicas)
	}
	if p.readCap < 63000 {
		t.Fatalf("read capacity %0.0f must cover AZ-redundant demand 63000", p.readCap)
	}
}

func TestShardWhenWriteWallHit(t *testing.T) {
	in := base()
	in.writeTPS = 5000 // 单主安全写 4000×70%=2800
	p := compute(in)
	if !p.sharded {
		t.Fatal("write 5000 TPS must trigger sharding")
	}
	if len(p.blockers) == 0 {
		t.Fatal("write wall must be reported as a blocker")
	}
}

func TestSmallSystemStaysSingle(t *testing.T) {
	in := base()
	in.readQPS, in.writeTPS, in.rows, in.dataGB = 500, 20, 1e6, 5
	in.rtoSec = 3600 // 能接受小时级停机 → 不需要常备副本
	p := compute(in)
	if p.topology != "单机" {
		t.Fatalf("want 单机, got %s", p.topology)
	}
}

func TestCostAndFanout(t *testing.T) {
	in := base()
	p := compute(in)
	if want := float64(1+p.replicas) * in.pricePerNode; p.costMonth != want {
		t.Fatalf("cost month = %0.0f, want %0.0f", p.costMonth, want)
	}
	if want := float64(p.replicas) * in.binlogMBps; p.fanoutMBps != want {
		t.Fatalf("fanout = %0.0f, want %0.0f", p.fanoutMBps, want)
	}
}

func TestRPOMustBeZeroForSyncReplication(t *testing.T) {
	in := base()
	in.rows, in.dataGB, in.readQPS = 5e6, 100, 30000 // 不触发分片，只看复制模式
	if p := compute(in); p.topology == "" || !contains(p.topology, "半同步") {
		t.Fatalf("RPO=0 should choose 半同步, got %q", p.topology)
	}
	in.rpoSec = 30
	if p := compute(in); contains(p.topology, "半同步") {
		t.Fatalf("RPO=30s should not pay for 半同步, got %q", p.topology)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
