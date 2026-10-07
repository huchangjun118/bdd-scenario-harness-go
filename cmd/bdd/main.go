// Command bdd 是本进程内的 BDD 场景运行器命令行入口。
// 它解析场景描述文件、绑定内置演示步骤、并发执行并输出报告。
// 真实项目应以库形式使用 bdd.Registry 注册自己的步骤定义，
// 参考 README 的示例。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	bdd "github.com/huchangjun118/bdd-scenario-harness-go"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("bdd", flag.ContinueOnError)
	var (
		tags        = fs.String("tags", "", "只运行同时具备这些标签的场景（逗号分隔）")
		excludeTags = fs.String("exclude-tags", "", "排除带这些标签的场景（逗号分隔）")
		name        = fs.String("name", "", "只运行名称包含该子串的场景")
		concurrency = fs.Int("concurrency", 4, "并发执行的场景数上限")
		format      = fs.String("format", "pretty", "输出格式：pretty 或 json")
		report      = fs.String("report", "", "把 JSON 报告写入指定文件")
	)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "用法: bdd [选项] <feature文件或目录>...\n\n选项:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		fs.Usage()
		return 2
	}

	features, err := loadFeatures(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析失败:", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := &bdd.Runner{
		Registry:    demoRegistry(),
		Concurrency: *concurrency,
		Filter: bdd.Filter{
			NameContains: *name,
			IncludeTags:  splitCSV(*tags),
			ExcludeTags:  splitCSV(*excludeTags),
		},
	}
	res := runner.Run(ctx, features)

	if *format == "json" {
		if err := bdd.WriteJSONReport(os.Stdout, res); err != nil {
			fmt.Fprintln(os.Stderr, "写报告失败:", err)
			return 2
		}
	} else {
		printPretty(os.Stdout, res)
	}
	if *report != "" {
		f, err := os.Create(*report)
		if err != nil {
			fmt.Fprintln(os.Stderr, "创建报告文件失败:", err)
			return 2
		}
		defer f.Close()
		if err := bdd.WriteJSONReport(f, res); err != nil {
			fmt.Fprintln(os.Stderr, "写报告失败:", err)
			return 2
		}
	}

	if res.Cancelled {
		fmt.Fprintln(os.Stderr, "运行被中途取消，已保留完成部分")
		return 130
	}
	if res.Failed() {
		return 1
	}
	return 0
}

// loadFeatures 读取文件或目录（递归 *.feature）并解析；
// 任何一个文件解析失败都整体失败，不产出半截结果。
func loadFeatures(paths []string) ([]*bdd.Feature, error) {
	var files []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			files = append(files, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".feature") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var features []*bdd.Feature
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		feat, err := bdd.Parse(f, src)
		if err != nil {
			return nil, err
		}
		features = append(features, feat)
	}
	return features, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// demoRegistry 提供一组内置演示步骤，使 CLI 可以端到端运行。
func demoRegistry() *bdd.Registry {
	r := bdd.NewRegistry()
	must := func(expr string, fn bdd.StepFunc) {
		if err := r.Register(expr, fn); err != nil {
			panic(err)
		}
	}
	must("a passing step", func(ctx *bdd.StepContext, args ...any) error { return nil })
	must("a failing step", func(ctx *bdd.StepContext, args ...any) error {
		return errors.New("this step always fails")
	})
	must("a pending step", func(ctx *bdd.StepContext, args ...any) error { return bdd.ErrPending })
	must("I wait {int} ms", func(ctx *bdd.StepContext, args ...any) error {
		ms := args[0].(int64)
		select {
		case <-ctx.Context.Done():
			return ctx.Context.Err()
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return nil
		}
	})
	must("I store {string} as {word}", func(ctx *bdd.StepContext, args ...any) error {
		ctx.Set(args[1].(string), args[0].(string))
		return nil
	})
	must("{word} should equal {string}", func(ctx *bdd.StepContext, args ...any) error {
		v, ok := ctx.Get(args[0].(string))
		if !ok {
			return fmt.Errorf("no value stored under %q", args[0])
		}
		if v != args[1].(string) {
			return fmt.Errorf("expected %q, got %q", args[1], v)
		}
		return nil
	})
	return r
}

func printPretty(w *os.File, res *bdd.RunResult) {
	counts := map[bdd.Status]int{}
	for _, s := range res.Scenarios {
		counts[s.Status]++
		mark := map[bdd.Status]string{
			bdd.StatusPassed: "PASS", bdd.StatusFailed: "FAIL", bdd.StatusPending: "PEND",
			bdd.StatusUndefined: "UNDF", bdd.StatusAmbiguous: "AMBG",
			bdd.StatusSkipped: "SKIP", bdd.StatusCancelled: "CNCL",
		}[s.Status]
		fmt.Fprintf(w, "%s %s:%d %s\n", mark, s.FeaturePath, s.Line, s.Name)
		if s.SkipReason != "" {
			fmt.Fprintf(w, "     reason: %s\n", s.SkipReason)
		}
		for _, st := range s.Steps {
			if st.Status == bdd.StatusPassed {
				continue
			}
			fmt.Fprintf(w, "     [%s] %s %s\n", st.Status, st.Keyword, st.Text)
			if st.Error != "" {
				fmt.Fprintf(w, "       error: %s\n", st.Error)
			}
			for _, he := range st.HookErrors {
				fmt.Fprintf(w, "       hook: %s\n", he)
			}
		}
		for _, he := range s.HookErrors {
			fmt.Fprintf(w, "     hook: %s\n", he)
		}
	}
	fmt.Fprintf(w, "\n%d scenarios: %d passed, %d failed, %d pending, %d undefined, %d ambiguous, %d skipped, %d cancelled\n",
		len(res.Scenarios), counts[bdd.StatusPassed], counts[bdd.StatusFailed], counts[bdd.StatusPending],
		counts[bdd.StatusUndefined], counts[bdd.StatusAmbiguous], counts[bdd.StatusSkipped], counts[bdd.StatusCancelled])
}
