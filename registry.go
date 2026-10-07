package bdd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrPending 由步骤实现返回，表示该步骤被标记为“待实现”。
var ErrPending = errors.New("step is pending implementation")

// StepContext 在一次场景执行内共享，用于步骤间传递状态。
// 每个场景拥有独立的 StepContext，并发场景之间互不可见。
type StepContext struct {
	Context  context.Context
	Scenario *Scenario
	values   map[string]any
}

// Set 记录场景内的键值状态。
func (c *StepContext) Set(key string, v any) { c.values[key] = v }

// Get 读取场景内的键值状态。
func (c *StepContext) Get(key string) (any, bool) { v, ok := c.values[key]; return v, ok }

// StepFunc 是步骤实现函数。args 为模式匹配得到的参数（已按占位符类型转换：
// {string}/{word} 为 string，{int} 为 int64，{float} 为 float64）。
// 若步骤带 DocString 或 DataTable，会作为最后一个参数附加（*DocString 或 *DataTable）。
type StepFunc func(ctx *StepContext, args ...any) error

// Registry 保存步骤定义，并负责把场景步骤绑定到定义上。
type Registry struct {
	defs []*stepDef
	seen map[string]bool // 规范化骨架 -> 是否已注册
}

type stepDef struct {
	expr  string         // 原始模式
	re    *regexp.Regexp // 编译后的匹配表达式
	kinds []string       // 各捕获组的占位符类型
	fn    StepFunc
}

// NewRegistry 创建空的步骤注册表。
func NewRegistry() *Registry { return &Registry{seen: map[string]bool{}} }

// Register 注册一个步骤定义。模式支持 {string} {int} {float} {word} 占位符。
// 规范化后相同的重复模式会被拒绝并返回错误。
func (r *Registry) Register(expr string, fn StepFunc) error {
	if fn == nil {
		return fmt.Errorf("bdd: step %q registered with nil function", expr)
	}
	re, kinds, skeleton, err := compilePattern(expr)
	if err != nil {
		return err
	}
	if r.seen[skeleton] {
		return fmt.Errorf("bdd: duplicate step definition %q", expr)
	}
	r.seen[skeleton] = true
	r.defs = append(r.defs, &stepDef{expr: expr, re: re, kinds: kinds, fn: fn})
	return nil
}

var placeholderRe = regexp.MustCompile(`\{(string|int|float|word)\}`)

// compilePattern 把模式编译为正则，返回各占位符类型与规范化骨架。
func compilePattern(expr string) (*regexp.Regexp, []string, string, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, nil, "", fmt.Errorf("bdd: empty step pattern")
	}
	var re strings.Builder
	var kinds []string
	var skeleton strings.Builder
	re.WriteByte('^')
	last := 0
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(expr, -1) {
		re.WriteString(regexp.QuoteMeta(expr[last:m[0]]))
		skeleton.WriteString(expr[last:m[0]])
		kind := expr[m[2]:m[3]]
		kinds = append(kinds, kind)
		skeleton.WriteString("{" + kind + "}")
		switch kind {
		case "string":
			re.WriteString(`"([^"]*)"`)
		case "int":
			re.WriteString(`(-?\d+)`)
		case "float":
			re.WriteString(`(-?\d+(?:\.\d+)?)`)
		case "word":
			re.WriteString(`(\S+)`)
		}
		last = m[1]
	}
	re.WriteString(regexp.QuoteMeta(expr[last:]))
	skeleton.WriteString(expr[last:])
	re.WriteByte('$')
	return regexp.MustCompile(re.String()), kinds, skeleton.String(), nil
}

// binding 是一次成功的步骤绑定。
type binding struct {
	def  *stepDef
	args []any
}

var (
	errUndefined = errors.New("no step definition matches")
	errAmbiguous = errors.New("multiple step definitions match")
)

// bind 查找步骤文本对应的唯一定义并转换参数。
// 未找到返回 errUndefined；匹配到多个定义返回 errAmbiguous（执行前被拒绝）。
func (r *Registry) bind(text string) (*binding, error) {
	var matched []*binding
	for _, d := range r.defs {
		m := d.re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		args := make([]any, 0, len(d.kinds))
		for i, kind := range d.kinds {
			raw := m[i+1]
			switch kind {
			case "int":
				n, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("bdd: cannot convert %q to int for pattern %q: %w", raw, d.expr, err)
				}
				args = append(args, n)
			case "float":
				f, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					return nil, fmt.Errorf("bdd: cannot convert %q to float for pattern %q: %w", raw, d.expr, err)
				}
				args = append(args, f)
			default:
				args = append(args, raw)
			}
		}
		matched = append(matched, &binding{def: d, args: args})
	}
	switch len(matched) {
	case 0:
		return nil, errUndefined
	case 1:
		return matched[0], nil
	default:
		exprs := make([]string, 0, len(matched))
		for _, b := range matched {
			exprs = append(exprs, b.def.expr)
		}
		return nil, fmt.Errorf("%w: %q matches %v", errAmbiguous, text, exprs)
	}
}
