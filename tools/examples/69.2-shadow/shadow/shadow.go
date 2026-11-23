// Package shadow 演示「物模型校验 + 设备影子」的最小可运行实现。
// 设计取舍：delta 用扁平路径（/power、/target/temp）而不是嵌套 JSON——
// 设备端解析成本低，MQTT 下行报文也小。
package shadow

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// DataType 只覆盖 IoT 常用标量；struct/array 在真实 TSL 里存在，但影子 diff 逻辑相同。
type DataType struct {
	Type  string         `json:"type"`            // bool / int32 / double / string / enum
	Specs map[string]any `json:"specs,omitempty"` // enum: {"values":["on","off"]}
}

type Property struct {
	Identifier string   `json:"identifier"`
	Name       string   `json:"name"`
	DataType   DataType `json:"dataType"`
	Access     string   `json:"access"` // r=只上报 / rw=可下发可上报
}

type ThingModel struct {
	Schema     string     `json:"schema"`
	Properties []Property `json:"properties"`
}

func (m *ThingModel) find(id string) (Property, bool) {
	for _, p := range m.Properties {
		if p.Identifier == id {
			return p, true
		}
	}
	return Property{}, false
}

var (
	ErrUnknownProperty = errors.New("物模型里没有这个属性")
	ErrReadOnly        = errors.New("属性是只读的（设备上报），不接受下行")
)

// ValidatePatch 用物模型校验一次下行/上行补丁：未知属性、只读属性、类型不符都拒绝。
// 注意：JSON 解出来的数字是 float64，int32 要额外判整数与范围，否则 "power":1.5 会被放过。
func (m *ThingModel) ValidatePatch(patch map[string]any) error {
	for id, v := range patch {
		p, ok := m.find(id)
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownProperty, id)
		}
		if p.Access == "r" {
			return fmt.Errorf("%w: %s", ErrReadOnly, id)
		}
		if err := checkType(p.DataType, v); err != nil {
			return fmt.Errorf("属性 %s: %w", id, err)
		}
	}
	return nil
}

// checkRange 读 specs 里的 min/max（TSL 里常写成字符串），越界即拒绝。
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
			if _, err := fmt.Sscanf(x, "%g", &lim); err != nil {
				return fmt.Errorf("specs.%s 不是数字: %v", k, x)
			}
		default:
			return fmt.Errorf("specs.%s 类型不支持: %T", k, raw)
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

func checkType(dt DataType, v any) error {
	switch dt.Type {
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("期望 bool，得到 %T", v)
		}
	case "int32":
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("期望数字，得到 %T", v)
		}
		if f != float64(int32(f)) {
			return fmt.Errorf("期望 int32 整数，得到 %v", f)
		}
		return checkRange(dt, f)
	case "double":
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("期望数字，得到 %T", v)
		}
		return checkRange(dt, f)
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("期望 string，得到 %T", v)
		}
	case "enum":
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("期望 enum 字符串，得到 %T", v)
		}
		vals, _ := dt.Specs["values"].([]any)
		hit := false
		for _, x := range vals {
			if s == x {
				hit = true
				break
			}
		}
		if !hit {
			return fmt.Errorf("不在枚举 %v 里：%q", vals, s)
		}
	default:
		return fmt.Errorf("未知数据类型 %q", dt.Type)
	}
	return nil
}

// ValidateReport 校验设备上报：属性必须存在、类型必须对；只读属性允许上报。
func (m *ThingModel) ValidateReport(patch map[string]any) error {
	for id, v := range patch {
		p, ok := m.find(id)
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownProperty, id)
		}
		if err := checkType(p.DataType, v); err != nil {
			return fmt.Errorf("属性 %s: %w", id, err)
		}
	}
	return nil
}

// Delta 是一条「期望值与上报值不一致」的记录，Path 是扁平路径。
type Delta struct {
	Path    string `json:"path"`
	Desired any    `json:"desired"`
}

var ErrVersionConflict = errors.New("影子版本冲突：有人先改了 desired")

type Shadow struct {
	mu       sync.Mutex
	model    *ThingModel
	desired  map[string]any
	reported map[string]any
	version  int64
}

func New(model *ThingModel) *Shadow {
	return &Shadow{model: model, desired: map[string]any{}, reported: map[string]any{}}
}

func (s *Shadow) Version() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// ApplyDesired 是「云端/App 改期望值」的唯一入口：
// ① 先做版本 CAS（expectedVersion=0 表示不校验），② 物模型校验，③ 合并，④ 回 delta。
func (s *Shadow) ApplyDesired(patch map[string]any, expectedVersion int64) (int64, []Delta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedVersion != 0 && expectedVersion != s.version {
		return s.version, nil, fmt.Errorf("%w: 期望 %d，实际 %d", ErrVersionConflict, expectedVersion, s.version)
	}
	if err := s.model.ValidatePatch(patch); err != nil {
		return s.version, nil, err
	}
	merge(s.desired, patch)
	s.version++
	return s.version, s.deltaLocked(), nil
}

// Report 是设备上报的入口：写入 reported，并返回「还差哪些属性没对齐」。
func (s *Shadow) Report(patch map[string]any) ([]Delta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.model.ValidateReport(patch); err != nil {
		return nil, err
	}
	merge(s.reported, patch)
	s.version++
	return s.deltaLocked(), nil
}

// Delta 给设备重连时调用：把离线期间攒下的期望值差量一次拉走。
func (s *Shadow) Delta() []Delta {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deltaLocked()
}

func (s *Shadow) deltaLocked() []Delta {
	out := []Delta{}
	flatten("", s.desired, s.reported, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// flatten 递归比对 desired 与 reported：desired 有、reported 缺或不等 → 一条 delta。
// 只关心 desired 里有的键——reported 多出来的属性不是「要下发的东西」。
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

// Snapshot 用于把影子落库（Redis/MySQL）后跨实例恢复；json 里的 version 必须一起存。
type Snapshot struct {
	Version  int64          `json:"version"`
	Desired  map[string]any `json:"desired"`
	Reported map[string]any `json:"reported"`
}

func (s *Shadow) Dump() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(Snapshot{Version: s.version, Desired: s.desired, Reported: s.reported})
}

func (s *Shadow) Load(b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	if snap.Desired == nil {
		snap.Desired = map[string]any{}
	}
	if snap.Reported == nil {
		snap.Reported = map[string]any{}
	}
	s.version, s.desired, s.reported = snap.Version, snap.Desired, snap.Reported
	return nil
}
