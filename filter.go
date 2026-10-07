package bdd

import "strings"

// SkipReason 区分场景未被执行的原因。
type SkipReason string

const (
	ReasonNotSelected SkipReason = "not_selected" // 未匹配筛选条件
	ReasonExcluded    SkipReason = "excluded"     // 被排除标签命中
	ReasonSkipped     SkipReason = "skipped"      // 主动跳过（@skip 标签）
)

// Filter 描述场景筛选条件。
type Filter struct {
	NameContains string   // 场景名包含该子串（不区分大小写，空为不限制）
	IncludeTags  []string // 必须全部具备的标签
	ExcludeTags  []string // 命中任一即排除
}

// Select 判断场景是否被选中；未选中时返回具体原因。
// 带 @skip 标签的场景视为选中，由执行器以 Skipped 状态报告。
func (f Filter) Select(s *Scenario) (bool, SkipReason) {
	tags := map[string]bool{}
	for _, t := range s.Tags {
		tags[t] = true
	}
	for _, t := range f.ExcludeTags {
		if tags[strings.TrimPrefix(t, "@")] {
			return false, ReasonExcluded
		}
	}
	for _, t := range f.IncludeTags {
		if !tags[strings.TrimPrefix(t, "@")] {
			return false, ReasonNotSelected
		}
	}
	if f.NameContains != "" &&
		!strings.Contains(strings.ToLower(s.Name), strings.ToLower(f.NameContains)) {
		return false, ReasonNotSelected
	}
	return true, ""
}

// HasTag 报告场景是否带指定标签（@ 前缀可选）。
func HasTag(s *Scenario, tag string) bool {
	tag = strings.TrimPrefix(tag, "@")
	for _, t := range s.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
