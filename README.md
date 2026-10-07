# bdd-scenario-harness-go

行为驱动测试（BDD）的场景编排与执行内核。团队用自然语言描述验收行为，
本地反复编辑并运行，结果稳定可复现。整条链路只在本进程内运行，
不依赖任何外部服务或真实凭据。

## 能力总览

- 解析自然语言场景描述文件（Feature / Scenario / Scenario Outline /
  Examples / Doc String / Data Table / 标签 / 注释 / Feature 描述）。
- 语法错误带 **文件、行、列** 与可读原因；解析失败不产出半截结果。
- 场景大纲按 Examples 展开为独立场景，逐示例独立运行、独立报告。
- 步骤绑定支持带类型参数的占位符模式；重复与歧义绑定在执行前被拒绝。
- 场景级与步骤级钩子，顺序确定，钩子异常不吞掉原始失败原因。
- 按标签与名称筛选；未选中、被排除、主动跳过是三种可区分的原因。
- 可配置并发度；场景间状态隔离；结果顺序与并发度无关，重复运行逐项一致。
- 机器可读 JSON 报告；存在失败/未实现/未定义时以非零退出码结束；
  运行被中途取消时保留已完成部分。

## 场景文件语法

```gherkin
@fast
Feature: 购物车
  Feature 标题下的自由文本是描述。

  @web
  Scenario: 加入商品
    Given 一个空购物车
    When 我加入一件商品
    Then 数量是 1

  Scenario Outline: 多件商品
    Given 一个空购物车
    When 我加入 <n> 件商品
    Then 数量是 <n>
    # 注释以 # 开头
    Examples:
      | n |
      | 2 |
      | 3 |
```

步骤下可挂文档字符串与数据表格：

```gherkin
    Given 请求体
      """json
      {"a": 1}
      """
    When 提交以下用户
      | name | age |
      | ann  | 30  |
```

语法错误示例输出：

```
解析失败: bad.feature:3:5: step "Given" requires text
```

## 步骤绑定语义

步骤定义通过 `Registry.Register` 注册，模式为字面文本加占位符：

| 占位符     | 匹配内容           | 转换后的 Go 类型 |
|------------|--------------------|------------------|
| `{string}` | `"..."` 双引号串   | `string`         |
| `{int}`    | 整数（可负）       | `int64`          |
| `{float}`  | 小数（可负）       | `float64`        |
| `{word}`   | 非空白词           | `string`         |

```go
reg := bdd.NewRegistry()
err := reg.Register(`用户 {string} 今年 {int} 岁`,
    func(ctx *bdd.StepContext, args ...any) error {
        name := args[0].(string)
        age := args[1].(int64)
        _ = name; _ = age
        return nil
    })
```

绑定规则：

- **重复拒绝**：规范化后相同的模式（字面部分与各占位符类型都相同）
  在注册时返回错误。
- **歧义拒绝**：执行前绑定步骤文本时，若命中多于一个定义，
  该步骤以 `ambiguous` 状态失败，错误信息列出所有候选模式。
- **未定义**：没有定义命中时步骤状态为 `undefined`。
- **待实现**：步骤函数返回 `bdd.ErrPending` 时状态为 `pending`，
  与 `failed` 明确区分。
- 步骤带 Doc String 或 Data Table 时，作为最后一个参数
  （`*bdd.DocString` / `*bdd.DataTable`）附加给步骤函数。
- 步骤函数 panic 会被捕获并记为失败，不会中断整个运行。

`StepContext` 是每个场景独立的状态容器（`Set`/`Get`），
并发场景之间互不可见。

## 执行顺序与钩子

```go
runner := &bdd.Runner{Registry: reg, Concurrency: 4}
runner.Hooks.BeforeScenario = append(runner.Hooks.BeforeScenario, ...)
runner.Hooks.AfterScenario  = append(runner.Hooks.AfterScenario, ...)
runner.Hooks.BeforeStep     = append(runner.Hooks.BeforeStep, ...)
runner.Hooks.AfterStep      = append(runner.Hooks.AfterStep, ...)
```

单个场景内的确定顺序：

```
BeforeScenario(注册顺序) → 逐步: BeforeStep(注册顺序) → 步骤 → AfterStep(注册逆序)
→ AfterScenario(注册逆序)
```

- `After*` 钩子在成功、失败与中断路径上**都会执行**（逆序，LIFO）。
- 钩子收到的 `err` 参数是到目前为止的原始错误；钩子自身出错时，
  错误被**追加**到结果的 `HookErrors`，原始失败原因保留在 `Error`，
  失败链路完整。
- `BeforeScenario` 失败：场景主体不执行（步骤记 `skipped`），
  `AfterScenario` 仍执行，场景记 `failed`。
- 任一步骤未通过后，同场景后续步骤记 `skipped`。
- 场景大纲的每个示例是独立场景，某个示例失败不影响其他示例。

## 筛选与跳过原因

```go
runner.Filter = bdd.Filter{
    NameContains: "登录",          // 名称子串（不区分大小写）
    IncludeTags:  []string{"web"}, // 必须全部具备
    ExcludeTags:  []string{"slow"},// 命中任一即排除
}
```

未执行场景的状态都是 `skipped`，用 `skip_reason` 区分：

| skip_reason     | 含义                           |
|-----------------|--------------------------------|
| `not_selected`  | 不满足名称或 include 标签条件  |
| `excluded`      | 命中 exclude 标签              |
| `skipped`       | 场景带 `@skip` 标签，主动跳过  |

Feature 级标签会被场景继承，Examples 级标签会被展开场景继承。

## 并发与确定性

- `Concurrency <= 0` 表示串行；`N > 0` 表示最多 N 个场景并发。
- 每个场景有独立的 `StepContext`，状态不共享。
- 报告中的场景顺序固定为输入顺序，与并发度和完成先后无关；
  相同输入重复运行得到逐项一致的结论。
- `context` 取消（CLI 中为 SIGINT/SIGTERM）：已完成的场景完整保留，
  未开始的场景标记为 `cancelled`，`RunResult.Cancelled` 置位。

## 状态与退出码

场景/步骤状态：`passed`、`failed`、`pending`、`undefined`、
`ambiguous`、`skipped`、`cancelled`。

CLI 退出码：

| 退出码 | 含义                                             |
|--------|--------------------------------------------------|
| 0      | 无失败、无待实现、无未定义、无歧义               |
| 1      | 存在 failed / pending / undefined / ambiguous    |
| 2      | 用法错误或场景文件解析失败                       |
| 130    | 运行被信号中途取消（已完成部分已保留并输出）     |

## CLI 用法

```
bdd [选项] <feature文件或目录>...

选项：
  -tags string        只运行同时具备这些标签的场景（逗号分隔）
  -exclude-tags string 排除带这些标签的场景（逗号分隔）
  -name string        只运行名称包含该子串的场景
  -concurrency int    并发执行的场景数上限（默认 4）
  -format string      输出格式：pretty 或 json（默认 pretty）
  -report string      把 JSON 报告写入指定文件
```

目录参数会递归收集 `*.feature` 文件；任一文件解析失败即整体失败。

CLI 内置一组演示步骤（见 `cmd/bdd/main.go` 的 `demoRegistry`），
可直接运行 `examples/features/demo.feature`：

```
go run ./cmd/bdd examples/features/demo.feature
go run ./cmd/bdd --tags outline --format json examples/features/demo.feature
```

真实项目应以库形式使用：在自己的 `main` 中创建 `bdd.Registry`、
注册项目步骤定义、配置 `bdd.Runner` 并调用 `Run`。

## JSON 报告

`bdd.WriteJSONReport(w, res)` 输出：

```json
{
  "cancelled": false,
  "summary": {"total": 7, "passed": 4, "failed": 1, "pending": 1,
              "undefined": 0, "ambiguous": 0, "skipped": 1, "cancelled": 0},
  "scenarios": [
    {
      "feature": "演示内置步骤",
      "feature_path": "examples/features/demo.feature",
      "name": "明确失败", "line": 10, "example_index": -1,
      "status": "failed", "duration_ms": 0.01,
      "steps": [
        {"keyword": "Given", "text": "a failing step", "line": 11,
         "status": "failed", "duration_ms": 0.005,
         "error": "this step always fails"}
      ]
    }
  ]
}
```

## 测试

```
go test ./...
```

覆盖：语法错误定位、大纲展开、步骤绑定冲突（重复/歧义）、
钩子异常与失败链路、并发状态隔离、结果顺序确定性、
取消保留部分结果、筛选原因区分、JSON 报告。
