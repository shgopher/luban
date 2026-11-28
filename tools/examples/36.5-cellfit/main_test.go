package main

import (
	"math"
	"testing"
)

func base() input {
	return input{
		users: 5e6, cells: 4, crossPct: 0.75, lossPerMin: 20000,
		rtoNowMin: 240, rtoNewMin: 5, failsPerYear: 2,
		personMonths: 60, pmCost: 40000, infraMonth: 2e5, infraInc: 0.6,
		hops: 3, fastMs: 5, slowMs: 100, slowProb: 0.01, p99BudgetMs: 300,
		crossWrite: true, opsReady: true,
	}
}

func TestBlastRadiusAndLoss(t *testing.T) {
	in := base()
	r := compute(in)
	if r.radiusPct != 25 {
		t.Fatalf("4 cells must expose 25%% of users, got %0.1f", r.radiusPct)
	}
	// 现状：2 次 × 240 分 × 20000 元 = 960 万
	if want := 2.0 * 240 * 20000; r.lossNowYear != want {
		t.Fatalf("loss now = %.0f, want %.0f", r.lossNowYear, want)
	}
	// 单元化后：2 次 × 5 分 × 20000 × 1/4 = 5 万
	if want := 2.0 * 5 * 20000 / 4; r.lossCellYear != want {
		t.Fatalf("loss cell = %.0f, want %.0f", r.lossCellYear, want)
	}
	if r.yearGain <= 0 {
		t.Fatal("year gain must be positive for this case")
	}
}

func TestP99TailAmplification(t *testing.T) {
	in := base()
	// 3 跳串联、每跳 1% 概率慢：全快概率 0.99^3 = 97.03% < 99%，页面 P99 落到「有一跳慢」
	if got := compute(in).pageP99; got != 100+2*5 {
		t.Fatalf("page p99 = %0.0f, want 110", got)
	}
	// 1 跳时全快概率 99% ≥ 99%，P99 就是单跳快耗时
	in.hops = 1
	if got := compute(in).pageP99; got != 5 {
		t.Fatalf("1 hop p99 = %0.0f, want 5", got)
	}
}

func TestGateBlocksWithoutWriteConflict(t *testing.T) {
	in := base()
	in.crossWrite = false
	r := compute(in)
	if r.verdict != "现在别做" {
		t.Fatalf("no cross-region write conflict must block, got %q", r.verdict)
	}
	if r.gates[0].ok {
		t.Fatal("gate 1 should fail")
	}
}

func TestGateBlocksWhenP99OverBudget(t *testing.T) {
	in := base()
	in.p99BudgetMs = 80
	r := compute(in)
	if r.verdict != "现在别做" {
		t.Fatalf("p99 110ms > budget 80ms must block, got %q", r.verdict)
	}
}

func TestPaybackMonths(t *testing.T) {
	in := base()
	r := compute(in)
	want := r.oneTimeCost / (r.netGainYear / 12)
	if math.Abs(r.paybackMonths-want) > 1e-9 {
		t.Fatalf("payback = %.2f, want %.2f", r.paybackMonths, want)
	}
	if r.oneTimeCost != 60*40000+0.6*2e5*3 {
		t.Fatalf("one-time cost = %.0f", r.oneTimeCost)
	}
}
