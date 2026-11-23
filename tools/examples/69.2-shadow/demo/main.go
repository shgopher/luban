// 单文件可运行版：物模型校验 + 设备影子差量。跑法：go run main.go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
)

type DataType struct {
	Type  string         `json:"type"` // bool / int32 / double / string / enum
	Specs map[string]any `json:"specs,omitempty"`
}
type Property struct {
	Identifier string   `json:"identifier"`
	DataType   DataType `json:"dataType"`
	Access     string   `json:"access"` // r / rw
}
type ThingModel struct {
	Schema     string     `json:"schema"`
	Properties []Property `json:"properties"`
}

var errUnknown = errors.New("物模型里没有这个属性")

func (m *ThingModel) find(id string) (Property, bool) {
	for _, p := range m.Properties {
		if p.Identifier == id {
			return p, true
		}
	}
	return Property{}, false
}

// ValidatePatch：未知属性、只读属性下行、类型不符一律拒绝。
func (m *ThingModel) ValidatePatch(patch map[string]any) error {
	for id, v := range patch {
		p, ok := m.find(id)
		if !ok {
			return fmt.Errorf("%w: %s", errUnknown, id)
		}
		if p.Access == "r" {
			return fmt.Errorf("只读属性不接受下行: %s", id)
		}
		switch p.DataType.Type {
		case "bool":
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("属性 %s: 期望 bool，得到 %T", id, v)
			}
		case "double":
			f, ok := v.(float64)
			if !ok {
				return fmt.Errorf("属性 %s: 期望数字，得到 %T", id, v)
			}
			if err := checkRange(p.DataType, f); err != nil {
				return fmt.Errorf("属性 %s: %w", id, err)
			}
		case "int32":
			f, ok := v.(float64)
			if !ok || f != float64(int32(f)) {
				return fmt.Errorf("属性 %s: 期望 int32 整数，得到 %v", id, v)
			}
			if err := checkRange(p.DataType, f); err != nil {
				return fmt.Errorf("属性 %s: %w", id, err)
			}
		case "enum":
			vals, _ := p.DataType.Specs["values"].([]any)
			hit := false
			for _, x := range vals {
				if v == x {
					hit = true
				}
			}
			if !hit {
				return fmt.Errorf("属性 %s: 不在枚举 %v 里：%v", id, vals, v)
			}
		}
	}
	return nil
}

type Delta struct {
	Path    string `json:"path"`
	Desired any    `json:"desired"`
}

// checkRange：specs 里 min/max 常写成字符串，越界即拒绝。
func checkRange(dt DataType, f float64) error {
	for _, k := range []string{"min", "max"} {
		raw, ok := dt.Specs[k]
		if !ok {
			continue
		}
		var lim float64
		switch x := raw.(type) {
		case float64:
			lim = x
		case string:
			fmt.Sscanf(x, "%g", &lim)
		}
		if k == "min" && f < lim {
			return fmt.Errorf("低于下限 %v: %v", lim, f)
		}
		if k == "max" && f > lim {
			return fmt.Errorf("高于上限 %v: %v", lim, f)
		}
	}
	return nil
}

type Shadow struct {
	model    *ThingModel
	desired  map[string]any
	reported map[string]any
	version  int64
}

func New(m *ThingModel) *Shadow {
	return &Shadow{model: m, desired: map[string]any{}, reported: map[string]any{}}
}

// ApplyDesired：版本 CAS（expectedVersion=0 表示不校验）→ 校验 → 合并 → 回差量。
func (s *Shadow) ApplyDesired(patch map[string]any, expectedVersion int64) (int64, []Delta, error) {
	if expectedVersion != 0 && expectedVersion != s.version {
		return s.version, nil, fmt.Errorf("版本冲突：期望 %d，实际 %d", expectedVersion, s.version)
	}
	if err := s.model.ValidatePatch(patch); err != nil {
		return s.version, nil, err
	}
	merge(s.desired, patch)
	s.version++
	return s.version, s.delta(), nil
}

func (s *Shadow) Report(patch map[string]any) []Delta {
	for id := range patch {
		if _, ok := s.model.find(id); !ok {
			panic("设备上报了未定义属性: " + id)
		}
	}
	merge(s.reported, patch)
	s.version++
	return s.delta()
}

func (s *Shadow) delta() []Delta {
	out := []Delta{}
	flatten("", s.desired, s.reported, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func flatten(prefix string, desired, reported map[string]any, out *[]Delta) {
	for k, dv := range desired {
		path := k
		if prefix != "" {
			path = prefix + "/" + k
		}
		dm, dok := dv.(map[string]any)
		rv, present := reported[k]
		rm, rok := rv.(map[string]any)
		if dok && rok {
			flatten(path, dm, rm, out)
			continue
		}
		if !present || !reflect.DeepEqual(dv, rv) {
			*out = append(*out, Delta{Path: path, Desired: dv})
		}
	}
}

func merge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				merge(dm, sm)
				continue
			}
			nm := map[string]any{}
			merge(nm, sm)
			dst[k] = nm
			continue
		}
		dst[k] = v
	}
}

func main() {
	model := &ThingModel{Schema: "thing.v1", Properties: []Property{
		{Identifier: "power", DataType: DataType{Type: "bool"}, Access: "rw"},
		{Identifier: "target_temp", DataType: DataType{Type: "double", Specs: map[string]any{"min": "16", "max": "32"}}, Access: "rw"},
		{Identifier: "mode", DataType: DataType{Type: "enum", Specs: map[string]any{"values": []any{"cool", "heat", "auto"}}}, Access: "rw"},
		{Identifier: "current_temp", DataType: DataType{Type: "double"}, Access: "r"},
	}}
	s := New(model)

	// 场景 1：App 设置期望值；设备此刻离线，之后上报的仍是旧值
	v, d, err := s.ApplyDesired(map[string]any{"power": true, "target_temp": 26.0}, 0)
	fmt.Printf("① 下发期望后 version=%d delta=%s err=%v\n", v, must(d), err)

	// 场景 2：设备重连上报（power 已执行、温度没变）→ delta 只剩 target_temp
	d = s.Report(map[string]any{"power": true, "current_temp": 23.5})
	fmt.Printf("② 设备上报后 delta=%s\n", must(d))

	// 场景 3：两个 App 并发改，第二个拿旧版本必须失败
	_, _, err = s.ApplyDesired(map[string]any{"target_temp": 28.0}, 1)
	fmt.Printf("③ 旧版本写入 err=%v\n", err)

	// 场景 4：物模型拦截
	for _, bad := range []map[string]any{
		{"current_temp": 20.0}, // 只读属性不能下发
		{"no_such": 1.0},       // 未定义属性
		{"mode": "dry"},        // 枚举越界
		{"power": "true"},      // 类型不符
		{"target_temp": 80.0},  // 超出规格范围
	} {
		fmt.Printf("④ 拒绝 %v → %v\n", bad, s.model.ValidatePatch(bad))
	}
}

func must(d []Delta) string {
	b, _ := json.Marshal(d)
	return string(b)
}
