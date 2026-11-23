package shadow

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func testModel() *ThingModel {
	return &ThingModel{
		Schema: "thing.v1",
		Properties: []Property{
			{Identifier: "power", Name: "开关", DataType: DataType{Type: "bool"}, Access: "rw"},
			{Identifier: "target_temp", Name: "目标温度", DataType: DataType{Type: "double", Specs: map[string]any{"min": "16", "max": "32"}}, Access: "rw"},
			{Identifier: "mode", Name: "模式", DataType: DataType{Type: "enum", Specs: map[string]any{"values": []any{"cool", "heat", "auto"}}}, Access: "rw"},
			{Identifier: "brightness", Name: "亮度", DataType: DataType{Type: "int32"}, Access: "rw"},
			{Identifier: "current_temp", Name: "当前温度", DataType: DataType{Type: "double"}, Access: "r"},
		},
	}
}

// 用例 1：desired 与 reported 三态——只有不一致的才进 delta。
func TestDeltaThreeStates(t *testing.T) {
	s := New(testModel())
	if _, _, err := s.ApplyDesired(map[string]any{"power": true, "target_temp": 26.0}, 0); err != nil {
		t.Fatal(err)
	}
	// 设备上报 power 已对齐，target_temp 仍是旧值
	delta, err := s.Report(map[string]any{"power": true, "target_temp": 24.0, "current_temp": 23.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(delta) != 1 || delta[0].Path != "target_temp" || delta[0].Desired != 26.0 {
		t.Fatalf("delta 不对：%+v", delta)
	}
	b, _ := json.Marshal(delta)
	t.Logf("delta=%s", b) // 期望 [{"path":"target_temp","desired":26}]
}

// 用例 2：两个 App 同时改 desired，后到的版本 CAS 必须失败（不覆盖前一个）。
func TestVersionConflict(t *testing.T) {
	s := New(testModel())
	v1, _, err := s.ApplyDesired(map[string]any{"target_temp": 26.0}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ApplyDesired(map[string]any{"target_temp": 28.0}, v1); err != nil {
		t.Fatalf("同版本应成功：%v", err)
	}
	// 第二个写入者拿着旧版本再来一次
	_, _, err = s.ApplyDesired(map[string]any{"target_temp": 30.0}, v1)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("应报版本冲突，得到 %v", err)
	}
}

// 用例 3：设备离线时下发期望值，重连后只拿 delta；上报对齐后 delta 清空。
func TestOfflineReplay(t *testing.T) {
	s := New(testModel())
	// 设备离线：App 设置 power=on, mode=heat
	s.ApplyDesired(map[string]any{"power": true, "mode": "heat"}, 0)
	// 设备重连：先拉 delta（真实实现是 MQTT 主题 .../shadow/delta）
	d := s.Delta()
	if len(d) != 2 {
		t.Fatalf("重连应拉到 2 条 delta，得到 %+v", d)
	}
	// 设备执行后上报
	d2, err := s.Report(map[string]any{"power": true, "mode": "heat", "current_temp": 21.0})
	if err != nil {
		t.Fatal(err)
	}
	if len(d2) != 0 {
		t.Fatalf("对齐后 delta 应为空，得到 %+v", d2)
	}
}

// 用例 4：物模型校验——未知属性、只读属性、类型不符、枚举越界都要拒绝。
func TestThingModelValidation(t *testing.T) {
	m := testModel()
	cases := []struct {
		name  string
		patch map[string]any
	}{
		{"未知属性", map[string]any{"no_such": 1.0}},
		{"只读属性不能下行", map[string]any{"current_temp": 20.0}},
		{"int32 不能收小数", map[string]any{"brightness": 1.5}},
		{"超出规格范围", map[string]any{"target_temp": 80.0}},
		{"枚举越界", map[string]any{"mode": "dry"}},
		{"bool 类型不符", map[string]any{"power": "true"}},
	}
	for _, c := range cases {
		if err := m.ValidatePatch(c.patch); err == nil {
			t.Errorf("%s：应该报错却通过了", c.name)
		} else {
			t.Logf("%-16s → %v", c.name, err)
		}
	}
	// 上报允许只读属性
	if err := m.ValidateReport(map[string]any{"current_temp": 23.5}); err != nil {
		t.Errorf("上报只读属性应允许：%v", err)
	}
}

// 用例 5：并发写。100 个 App 抢同一版本、100 个设备上报，-race 下不能崩也不能串版本。
func TestConcurrent(t *testing.T) {
	s := New(testModel())
	var wg sync.WaitGroup
	var mu sync.Mutex
	var okCount int
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, _, err := s.ApplyDesired(map[string]any{"target_temp": float64(20 + i%10)}, 0); err == nil {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}(i)
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.Report(map[string]any{"current_temp": float64(i)}); err != nil {
				t.Errorf("report: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if okCount != 100 {
		t.Fatalf("100 次无版本校验的写入都应成功，实际 %d", okCount)
	}
	t.Logf("最终版本号 version=%d（100 次 desired + 100 次 report）", s.Version())
}
