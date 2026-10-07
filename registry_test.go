package bdd

import (
	"errors"
	"strings"
	"testing"
)

func noop(ctx *StepContext, args ...any) error { return nil }

func TestRegisterDuplicateRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("I have {int} items", noop); err != nil {
		t.Fatalf("first register: %v", err)
	}
	// 规范化骨架相同即重复，即使占位符类型名不同写法也一样拒绝
	if err := r.Register("I have {int} items", noop); err == nil {
		t.Fatal("duplicate pattern not rejected")
	}
	if err := r.Register("", noop); err == nil {
		t.Fatal("empty pattern not rejected")
	}
	if err := r.Register("x", nil); err == nil {
		t.Fatal("nil func not rejected")
	}
}

func TestBindTypeConversion(t *testing.T) {
	r := NewRegistry()
	var got []any
	must := func(expr string) {
		if err := r.Register(expr, func(ctx *StepContext, args ...any) error {
			got = args
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	must(`user {string} is {int} years old`)
	must(`ratio is {float}`)
	must(`say {word}`)

	b, err := r.bind(`user "ann" is 30 years old`)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.def.fn(nil, b.args...); err != nil {
		t.Fatal(err)
	}
	if got[0] != "ann" || got[1] != int64(30) {
		t.Errorf("args = %#v", got)
	}

	b, err = r.bind(`ratio is -2.5`)
	if err != nil {
		t.Fatal(err)
	}
	if b.args[0] != -2.5 {
		t.Errorf("float arg = %#v", b.args[0])
	}

	b, err = r.bind(`say hello`)
	if err != nil {
		t.Fatal(err)
	}
	if b.args[0] != "hello" {
		t.Errorf("word arg = %#v", b.args[0])
	}
}

func TestBindUndefinedAndAmbiguous(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("I have {int} items", noop)
	_ = r.Register("I have {word} items", noop) // 骨架不同，允许注册

	if _, err := r.bind("nothing matches here"); !errors.Is(err, errUndefined) {
		t.Errorf("undefined err = %v", err)
	}
	// "I have 3 items" 同时命中 {int} 与 {word} -> 歧义，执行前被拒绝
	_, err := r.bind("I have 3 items")
	if !errors.Is(err, errAmbiguous) {
		t.Fatalf("ambiguous err = %v", err)
	}
	if !strings.Contains(err.Error(), "I have {int} items") {
		t.Errorf("error should list candidates: %v", err)
	}
	// 只命中 {word} 的文本仍可绑定
	if _, err := r.bind("I have many items"); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}
