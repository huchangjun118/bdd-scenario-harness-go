package bdd

// ScenarioHook 在场景级执行。err 为场景主体已发生的错误（可能为 nil），
// 钩子返回的错误会被追加进失败链路而不覆盖原始错误。
type ScenarioHook func(ctx *StepContext, err error) error

// StepHook 在步骤级执行。step 为当前步骤，err 为该步骤已发生的错误。
type StepHook func(ctx *StepContext, step *Step, err error) error

// Hooks 收集场景级与步骤级钩子。
// 执行顺序固定：Before 按注册顺序执行，After 按注册逆序（LIFO）执行；
// After 钩子在成功、失败与中断路径上都会执行。
type Hooks struct {
	BeforeScenario []ScenarioHook
	AfterScenario  []ScenarioHook
	BeforeStep     []StepHook
	AfterStep      []StepHook
}
