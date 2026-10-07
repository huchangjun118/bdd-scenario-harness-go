// Package bdd 提供行为驱动测试的场景编排与执行内核：
// 解析自然语言场景文件、绑定步骤定义、执行钩子、并发运行并产出报告。
package bdd

// Feature 对应一个场景描述文件。
type Feature struct {
	Path        string   // 文件路径
	Name        string   // Feature 名称
	Description string   // Feature 标题下的自由文本描述
	Tags        []string // Feature 级标签（会继承到场景）
	Scenarios   []*Scenario
}

// Scenario 是一个可执行场景；由场景大纲展开而来时带有示例行数据。
type Scenario struct {
	Name    string
	Keyword string // "Scenario" 或 "Scenario Outline"
	Line    int    // 在文件中的起始行（1 起）
	Tags    []string
	Steps   []*Step

	// 大纲相关字段。
	Outline  bool        // 是否为场景大纲
	Examples []*Examples // 大纲的示例组（仅解析阶段使用）

	// 展开后的场景字段；普通场景 ExampleIndex 为 -1。
	ExampleIndex int               // 示例行序号（0 起）
	ExampleRow   map[string]string // 该场景对应的示例行
	OutlineLine  int               // 所属大纲起始行
}

// Examples 是场景大纲下的一组数据示例。
type Examples struct {
	Name   string
	Line   int
	Tags   []string
	Header []string   // 表头列名
	Rows   []TableRow // 数据行
}

// TableRow 是一行带行号的表格数据。
type TableRow struct {
	Line  int
	Cells []string
}

// Step 是场景中的一个步骤。
type Step struct {
	Keyword   string // Given/When/Then/And/But（保留原文）
	Text      string
	Line      int
	DocString *DocString
	Table     *DataTable
}

// DocString 是步骤下的多行文本。
type DocString struct {
	ContentType string
	Content     string
	Line        int
}

// DataTable 是步骤下的数据表格。
type DataTable struct {
	Rows [][]string
	Line int
}
