@demo
Feature: 演示内置步骤
  展示通过、失败、待实现、跳过与大纲展开。

  Scenario: 全部通过
    Given a passing step
    When I store "hello" as greeting
    Then greeting should equal "hello"

  Scenario: 明确失败
    Given a failing step

  Scenario: 尚未实现
    Given a pending step

  @skip
  Scenario: 主动跳过
    Given a passing step

  @outline
  Scenario Outline: 存储并读取 <key>
    When I store "<value>" as <key>
    Then <key> should equal "<value>"

    Examples:
      | key   | value  |
      | first | alpha  |
      | second| beta   |
      | third | gamma  |
