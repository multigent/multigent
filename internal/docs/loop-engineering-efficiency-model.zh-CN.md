# Loop Engineering 为什么能提升协作效率

日期：2026-08-27

## 一句话结论

loop engineering 的价值不在于替代人类凭空产生想法，而在于当需求、问题、反馈、验证和跟进的数量超过人类注意力时，Agent 能持续消化这些信号，并只把真正需要人类判断的部分推回来。

换句话说：

> loop engineering 提升效率的前提，不是 Agent 比人更聪明，而是 Agent 可以更便宜、更持续、更并发地处理大量可规范化工作。

## 第一性原理

一个团队的产出通常被这些变量限制：

- 有多少待处理信号：需求、Issue、PR、客服反馈、测试失败、线上告警、竞品信息。
- 这些信号有多少可以被规范化处理。
- 每个任务需要多少人类判断。
- 每个任务需要多少执行时间。
- 人类能投入多少连续注意力。
- Agent 能投入多少计算时间和并发 session。

人类的稀缺资源不是“会不会做”，而是：

- 注意力有限。
- 工作时间有限。
- 上下文切换成本高。
- 很难持续追踪大量小任务。
- 不适合长时间重复检查、同步、整理、复测。

loop engineering 解决的是：

- 不忘记。
- 不疲劳。
- 不需要等人提醒才继续。
- 可以在无人值守时持续检查和推进。
- 可以把低价值但必要的步骤自动做完。
- 可以把需要人类判断的内容整理成结构化材料。

所以 loop engineering 的本质是：

> 用 Agent 的持续注意力，替换人类在重复推进、检查、整理、跟进上的注意力消耗。

## 外部资料给我们的修正

外部对 loop engineering 的主流讨论有几个共识，值得补进 Multigent 的设计里。

### 1. 从“人提示 Agent”到“系统提示 Agent”

Addy Osmani 对 loop engineering 的定义很直接：人不再反复 prompt Agent，而是设计一个系统，由系统发现工作、分配工作、检查结果、记录状态并决定下一步。

这和 Multigent 的方向一致，但我们的文档原来更强调“人类注意力释放”，还不够强调“prompting 责任被系统化”。

对应到 Multigent：

- Attention Signal 负责发现值得关注的变化。
- Task / Workflow 负责把变化转成可追踪工作。
- Wakeup / Schedule 负责周期性提示 Agent。
- Context / Docs / Audit 负责把状态留在会话外。
- Human Gate 负责在关键节点把判断权交回人类。

### 2. 一个可靠 loop 不只是“定时醒来”

arXiv 论文把一个工程化 loop 的构件总结为：触发的 Agent runs、机器可检查的停止条件、持久状态文件、验证子 Agent、token budgets、明确的人类升级点。

这提醒我们：如果 Multigent 只有心跳和 IM 唤醒，还不够。真正可靠的 loop engineering 至少需要：

- **触发条件**：定时、消息、任务、PR、Issue、测试失败、外部信号。
- **停止条件**：任务完成、验证通过、达到预算、进入阻塞、需要人类判断。
- **持久状态**：任务日志、文档、decision、audit、context artifact，而不是只留在模型会话里。
- **验证机制**：测试、CI、lint、review、checker Agent、人类 gate。
- **预算控制**：token、时间、并发 session、最大重试次数。
- **升级路径**：什么时候找谁，提供什么决策材料。

### 3. Loop 是多层的

LangChain 把 loop 拆成多层：Agent 自己的工具调用循环、verification loop、event-driven loop，以及分析 trace 反过来优化系统的 hill-climbing loop。

这说明 Multigent 不应该只把 loop engineering 理解成“Agent 心跳”。更准确地说：

```text
Agent 执行 loop
  -> 任务验证 loop
  -> 外部事件 / Attention loop
  -> 组织流程 loop
  -> 系统自我改进 loop
```

我们当前已经有前四层的雏形，但第五层“系统自我改进”还比较弱。后续需要从 runs、audit、失败任务、打回记录中自动发现：

- 哪些 prompt 经常导致 Agent 越界。
- 哪些工具经常失败。
- 哪些流程节点定义不清。
- 哪些 human gate 反复出现。
- 哪些任务类型应该沉淀成 template 或 skill。

### 4. 安全 loop 需要小步、可观察、可停止

Kilo 的总结强调：好的 loop 不是让 Agent 一口气做完大事，而是明确目标、提供上下文、采取小的可逆动作、观察结果、根据反馈修正，并设置停止规则。

对应到 Multigent：

- 任务应该尽量拆成可验证阶段。
- Agent 每次 wakeup 不必强行完成整个任务。
- 每个节点都要有“继续 / 阻塞 / 请求确认 / 完成”的清晰判断。
- 高风险动作需要 human gate。
- 运行记录要能看到 Agent 为什么继续、为什么停止。

### 5. Human-in-the-loop 不是失败，而是设计点

IBM 和 LangChain 都强调，人类仍然负责产品意图、风险、架构、质量、安全和业务结果。

这和我们“高自治 / 半自治 / 低自治”的划分是一致的。loop engineering 不是把人完全拿掉，而是把人类放到更高杠杆的位置：

- 不让人反复催进度。
- 不让人手动整理材料。
- 不让人逐条看低风险信号。
- 让人只看结构化决策材料。
- 让人对关键风险负责。

## 对当前文档的不足判断

搜索后回看这份文档，我认为原文有几个不足：

1. **缺少停止条件**：原文讲了任务量和人类工时，但没有明确 loop 什么时候应该停。
2. **缺少验证 loop**：原文讲了 QA 和 human gate，但没有把测试、CI、reviewer agent、rubric 当成系统性的验证层。
3. **缺少状态 spine**：原文讲了上下文中心，但还没强调每次循环必须把状态写到会话外，避免模型 session 成为唯一记忆。
4. **缺少预算**：原文数学模型有工时，但没有把 token、最大运行时间、最大重试、并发上限作为 loop 的控制变量。
5. **缺少自我改进 loop**：原文提了质量反馈闭环，但还不够具体；应该把 run trace / audit / failure analysis 作为优化 prompt、tool、workflow 的输入。
6. **缺少机器可检查完成条件**：对高自治任务来说，能否机器检查是决定自治程度的核心变量。

这些不足不推翻原文结论，但说明我们要把 loop engineering 从“效率叙事”推进到“工程系统设计”。

## 三类任务

### 1. 高自治任务

高自治任务是最适合 loop engineering 的任务。

特征：

- 输入来源稳定。
- 处理规则相对明确。
- 结果容易验证。
- 风险较低。
- 不需要频繁人类判断。
- 可以批量处理。

例子：

- GitHub Issue 初筛。
- PR 初审和 CI 跟进。
- 测试回归。
- Sentry / 日志异常巡检。
- 客服问题分类。
- 内容素材整理。
- 竞品信息收集。
- 周期性状态汇报。

这类任务中，loop engineering 的收益很高，因为 Agent 可以持续消费 backlog。

### 2. 半自治任务

半自治任务是 Multigent 最核心的产品机会。

特征：

- 需要 Agent 长时间推进。
- 中间大部分步骤可以自动完成。
- 少数节点需要人类确认。
- 需要流程、Spec、审计和上下文沉淀。

例子：

- 插件 / Connector 接入。
- 常规 Bug 修复。
- 小到中型功能开发。
- 发版准备。
- 客户问题闭环。
- 技术方案草拟与实现。
- 文章草稿、素材整理、发布前审核。

这类任务中，loop engineering 的收益来自：

- Agent 自己推进中间步骤。
- 人类只在方向、风险、验收、发布等 gate 介入。
- Agent 能在等待人类期间处理其他任务。

### 3. 低自治任务

低自治任务不适合强行自动化。

特征：

- 判断比执行更重要。
- 输入模糊。
- 目标本身需要探索。
- 涉及战略、组织、架构或商业取舍。
- 人类经验和上下文仍是核心瓶颈。

例子：

- 产品方向选择。
- 技术架构重大取舍。
- 商业定价。
- 品牌定位。
- 组织分工。
- 大客户承诺。

这类任务中，loop engineering 的价值不是“替人做决定”，而是：

- 收集材料。
- 对比方案。
- 暴露风险。
- 形成决策备忘。
- 跟踪决策后的执行。

## 简化数学模型

先定义变量。

任务数量：

```text
N_high = 高自治任务数量
N_mid  = 半自治任务数量
N_low  = 低自治任务数量
```

每类任务的人类时间和 Agent 时间：

```text
H_high = 每个高自治任务需要的人类工时
A_high = 每个高自治任务需要的 Agent 工时

H_mid = 每个半自治任务需要的人类工时
A_mid = 每个半自治任务需要的 Agent 工时

H_low = 每个低自治任务需要的人类工时
A_low = 每个低自治任务需要的 Agent 工时
```

人类和 Agent 的可用能力：

```text
P_h = 人类并发数
T_h = 每个人每天可投入有效工时
E_h = 人类效率系数

P_a = Agent 并发 session 数
T_a = 每个 Agent session 每天可运行小时
E_a = Agent 效率系数
```

loop 控制变量：

```text
B_a = 每天可用 Agent token / 费用预算
C_a = 每个任务平均 Agent 成本
R_a = 每个任务最大重试次数
L_a = 每个任务最大运行时长
V = 验证覆盖度，0 到 1
S = 停止条件清晰度，0 到 1
```

不使用 loop engineering 时，总人类工时近似为：

```text
HumanHours_no_loop =
  N_high * H_high_no_loop +
  N_mid  * H_mid_no_loop +
  N_low  * H_low_no_loop
```

使用 loop engineering 后，总人类工时近似为：

```text
HumanHours_with_loop =
  N_high * H_high_with_loop +
  N_mid  * H_mid_with_loop +
  N_low  * H_low_with_loop
```

Agent 工时为：

```text
AgentHours_with_loop =
  N_high * A_high +
  N_mid  * A_mid +
  N_low  * A_low
```

人类完成时间受人类有效产能限制：

```text
Days_no_loop =
  HumanHours_no_loop / (P_h * T_h * E_h)
```

使用 loop engineering 后，完成时间由人类瓶颈和 Agent 瓶颈共同决定：

```text
Days_with_loop =
  max(
    HumanHours_with_loop / (P_h * T_h * E_h),
    AgentHours_with_loop / (P_a * T_a * E_a)
  )
```

但实际系统还会受到预算约束：

```text
BudgetLimitedTasks = B_a / C_a
```

如果 `BudgetLimitedTasks` 小于待处理任务量，loop engineering 的瓶颈会从人类注意力转移到 Agent 预算。

同时，自治程度可以粗略看：

```text
AutonomyScore =
  Standardization
  * ToolCoverage
  * ContextAvailability
  * V
  * S
  * RiskControllability
```

其中 `V` 是验证覆盖度，`S` 是停止条件清晰度。没有验证和停止条件的 loop，很容易变成无限重试或错误自信。

loop engineering 的价值可以粗略表示为：

```text
Speedup = Days_no_loop / Days_with_loop
```

这不是精确预测，而是帮助我们判断一个场景是否值得自动化。

## 一个理想化例子

假设团队一天有：

```text
N_high = 40 个高自治任务
N_mid  = 10 个半自治任务
N_low  = 3 个低自治任务
```

不使用 loop engineering：

```text
H_high_no_loop = 0.25h
H_mid_no_loop  = 2h
H_low_no_loop  = 4h

HumanHours_no_loop =
  40 * 0.25 + 10 * 2 + 3 * 4
  = 42h
```

如果 2 个人每天各有 5 小时有效时间：

```text
P_h = 2
T_h = 5h
E_h = 1

Days_no_loop = 42 / (2 * 5) = 4.2 天
```

使用 loop engineering 后：

```text
H_high_with_loop = 0.03h  # 只看异常或摘要
H_mid_with_loop  = 0.4h   # 只做关键确认
H_low_with_loop  = 3h     # 仍主要靠人判断

A_high = 0.2h
A_mid  = 2.5h
A_low  = 1.5h
```

人类工时：

```text
HumanHours_with_loop =
  40 * 0.03 + 10 * 0.4 + 3 * 3
  = 14.2h
```

Agent 工时：

```text
AgentHours_with_loop =
  40 * 0.2 + 10 * 2.5 + 3 * 1.5
  = 37.5h
```

假设有 6 个 Agent session，每天可运行 8 小时，效率系数 0.7：

```text
P_a = 6
T_a = 8h
E_a = 0.7

AgentCapacity = 6 * 8 * 0.7 = 33.6h / day
```

完成时间：

```text
HumanBottleneck = 14.2 / 10 = 1.42 天
AgentBottleneck = 37.5 / 33.6 = 1.12 天

Days_with_loop = max(1.42, 1.12) = 1.42 天
Speedup = 4.2 / 1.42 = 2.96x
```

这个例子说明：

- loop engineering 不是消灭人类工作。
- loop engineering 是把人类从 42 小时降到 14.2 小时。
- 只要 Agent 产能足够，瓶颈会回到少数关键人类判断。
- 这时系统真正要优化的是 human gate 的质量和频率。

## 什么时候 loop engineering 最有效

### 高输入量

如果每天只有 1 个任务，loop engineering 的价值有限。

如果每天有几十个 issue、PR、客服问题、测试失败、外部信号，loop engineering 的价值会迅速放大。

### 高规范化

任务越能被描述成固定流程、固定输入、固定输出、固定验收，Agent 越能自主推进。

例如：

- “检查 PR 是否可合并”比“判断公司战略方向”更适合 loop engineering。
- “按插件接入规范接入 Monday”比“重新设计产品定位”更适合 loop engineering。

### 低风险自动执行

如果失败成本低，Agent 可以大胆做。

如果失败成本高，Agent 仍然可以做前置调研和方案整理，但关键动作要 human gate。

### 人类判断点可压缩

loop engineering 最大价值来自把人类介入压缩为：

- 是否做。
- 方向是否对。
- 风险是否可接受。
- 是否发布。
- 是否对外承诺。

而不是每一步都问人。

## 什么时候 loop engineering 价值较低

loop engineering 价值较低的场景：

- 没有稳定任务来源。
- 每个任务都高度定制。
- 任务需要密集实时讨论。
- 人类不愿意授权 Agent 做中间步骤。
- 没有清晰验收标准。
- Agent 工具不足，很多步骤只能问人。

这时更好的产品形态可能不是 loop engineering，而是：

- 深度对话。
- 方案评审。
- 结对思考。
- 一次性并发 session 执行。

## 能不能把更多任务变成高自治任务

可以，这是 Multigent 最重要的产品方向之一。

把任务从低自治提高到半自治、高自治，依赖这些能力：

### 1. Spec 模板

把模糊输入整理成结构化任务：

- 背景。
- 目标。
- 非目标。
- 影响范围。
- 验收标准。
- 风险。
- 相关上下文。

Spec 越稳定，Agent 越不需要反复问人。

### 2. 流程协议

流程不是僵硬自动化，而是协作规范：

- 什么时候该调研。
- 什么时候该写方案。
- 什么时候该开发。
- 什么时候该 QA。
- 什么时候必须找人。
- 输出什么字段才算完成。

流程把“人类经验”变成 Agent 可以遵循的工作协议。

### 3. 工具完备度

Agent 自主性受工具限制。

如果 Agent 没有 GitHub、CI、日志、Sentry、测试环境、部署状态、IM、知识库等工具，它就只能频繁问人。

工具越完整，任务越能自治。

### 4. 上下文中心

很多阻塞来自 Agent 不知道背景。

上下文中心应该沉淀：

- 项目背景。
- 历史决策。
- 架构文档。
- 会议纪要。
- 群聊讨论。
- 客户反馈。
- PR / Issue 历史。
- Agent 过去的经验。

上下文越完整，Agent 越少问重复问题。

### 5. 权限和审计

高自治不是无限授权。

要让人敢放权，必须有：

- 明确权限边界。
- 高风险动作 human gate。
- 全流程审计。
- 可追踪任务记录。
- 可回滚或可停止机制。

没有这些，Agent 越自治越危险。

### 6. 质量反馈闭环

Agent 做得不好时，需要知道为什么不好。

系统应沉淀：

- 哪些任务失败。
- 哪些节点经常打回。
- 哪些 Spec 字段缺失。
- 哪些 Agent 经常越界。
- 哪些工具调用失败。
- 哪些 human gate 重复出现。

这些反馈可以反过来优化 prompt、流程、工具和上下文。

### 7. 机器可检查停止条件

高自治任务的关键不是“Agent 愿意一直做”，而是系统知道它什么时候该停。

常见停止条件：

- 测试、lint、typecheck、build 全部通过。
- CI 全绿。
- PR 无冲突且 review 通过。
- 指定文档生成完成且字段完整。
- 客服回复草稿已生成并通过审核。
- 达到最大重试次数。
- 达到 token / 时间预算。
- Agent 判断需要人类决策，进入 human gate。

停止条件越明确，任务越容易从半自治升级为高自治。

### 8. 验证者和 maker/checker 结构

单个 Agent 自己检查自己的输出，容易自证正确。

更可靠的 loop engineering 应该支持 maker/checker：

- Maker Agent 负责实现。
- Checker Agent 负责测试、review、找边界问题。
- Human 只看 checker 整理出的关键风险和结论。

这对 PR review、测试、发版和客户回复尤其重要。

### 9. 状态 spine

长期 loop 不能依赖单个模型 session 记住所有事。

每次循环结束都应该把关键状态写到会话外：

- 当前目标。
- 已完成动作。
- 验证结果。
- 失败原因。
- 下一步。
- 是否需要人类。
- 相关文档、PR、Issue、消息、运行记录。

Multigent 里的任务日志、知识库文档、run trace、audit、context artifact 都应该共同承担这个 spine。

## 对 Multigent 的产品启发

### 1. 不要只做聊天

纯聊天只能提升局部效率，不能形成可运营组织。

Multigent 必须把聊天、任务、流程、权限、上下文、审计连起来。

### 2. 不要只做流程自动化

如果流程太硬，Agent 会被降级为工具函数。

正确方向是弱流程：

- 流程定义协议。
- Agent 保持判断。
- 人类只在关键点介入。

### 3. 不要只做个人 Agent

人手一个 Agent 只能提升个人生产力。

企业真正需要的是：

- 多 Agent 分工。
- 多人协作。
- 任务可追踪。
- 权限可控。
- 知识可沉淀。
- 流程可复用。

### 4. 优先落地高自治和半自治场景

早期最适合的客户场景：

- GitHub Issue / PR 流程。
- 客服反馈处理。
- 插件 / Connector 接入。
- 测试回归。
- 发版准备。
- 内容运营素材收集和初稿。
- 线上问题巡检。

这些场景能最快证明 loop engineering 的价值。

## 可量化指标

为了判断 loop engineering 是否真的有效，可以记录这些指标：

- 每天新增任务数。
- Agent 自动处理任务数。
- 人类介入次数。
- 每个任务平均人类工时。
- 每个任务平均 Agent 工时。
- 从创建到完成的周期。
- Human gate 平均等待时间。
- 任务打回率。
- 任务失败率。
- Agent 自主推进节点数。
- 人类确认后继续推进成功率。
- 每周被 Agent 消化的 backlog 数量。
- 机器可检查完成率。
- 达到停止条件的平均循环次数。
- 因预算停止的任务比例。
- 因信息不足升级给人类的比例。
- maker/checker 分歧率。
- 同类失败重复出现次数。

更直接的商业指标：

- 同样人数下处理了多少更多任务。
- 同样任务量下节省了多少人类工时。
- 是否减少了 owner / manager 的日常跟进时间。
- 是否缩短了 issue / PR / 客服问题的闭环周期。

## 一个判断公式

某个场景是否值得做 loop engineering，可以粗略看：

```text
LoopValue =
  TaskVolume
  * Standardization
  * HumanTimeReduction
  * OutcomeVerifiability
  * RiskControllability
```

其中：

- `TaskVolume`：任务量是否足够大。
- `Standardization`：流程和输入输出是否能规范化。
- `HumanTimeReduction`：能减少多少人类工时。
- `OutcomeVerifiability`：结果是否容易验证。
- `RiskControllability`：失败风险是否可控。

如果一个场景任务量小、不可规范化、结果难验证、风险高，那就不适合先做 loop engineering。

如果一个场景任务量大、流程稳定、结果可验、人类只需少量确认，那就是最优先场景。

## 结论

loop engineering 的意义不在于让 Agent 替代人类产生所有想法。

它真正解决的是：

- 大量信号没人持续看。
- 大量任务没人持续推。
- 大量中间步骤消耗人类注意力。
- 大量经验没有沉淀。
- 大量流程依赖人手动协调。

所以 Multigent 应该优先把任务从“人类持续推动”变成“Agent 持续推进，人类关键确认”。

当高自治和半自治任务足够多时，loop engineering 会显著提升团队吞吐；当任务仍然主要依赖少数人的判断时，loop engineering 的价值会下降，但仍然可以作为调研、整理、跟进和决策辅助系统存在。

最终目标不是让 Agent 变成被动工具，也不是让 Agent 无边界自由行动，而是建立一套可运营、可审计、可扩展的 Agent workforce：

> 人类负责方向、边界和关键判断；Agent 负责持续消化信号、推进任务、整理上下文，并在需要时把决策交还给人类。

## 参考来源

- Addy Osmani, “Loop Engineering”, 2026-06-07：提出 loop engineering 是把“反复提示 Agent”的责任系统化。
- LangChain, “The Art of Loop Engineering”, 2026-06-16：把 loop 拆成 Agent loop、verification loop、event-driven loop、hill-climbing loop，并强调 human oversight。
- Kilo, “What Is Loop Engineering? AI Feedback Loops”, 2026-06-10：总结 intent / context / action / observation / adjustment，以及小步、可观察、可停止。
- IBM, “What Is Loop Engineering?”, 2026-07-17：强调 skills、subagents、maker/checker、spine、人类治理。
- arXiv:2608.21884, “Loop Engineering: Building Blocks, Adoption, and Impact”, 2026-08-22：总结工程化 loop 的构件，包括触发 runs、机器可检查停止条件、持久状态、验证子 Agent、预算和人工升级点。
