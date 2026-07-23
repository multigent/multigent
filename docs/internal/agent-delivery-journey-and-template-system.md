# Agent Delivery Journey And Template System

本文档重新梳理 Multigent 的用户旅程、模板系统和 agent 交付闭环。目标不是再增加一组概念，而是让一个不了解 agent workflow 的团队，也能从空 workspace 走到第一个可持续交付的 agent。

## 核心判断

Multigent 下一阶段最重要的问题不是“有没有 team、project、agent、skill、credential、scheduler”，而是：

```text
用户如何把这些能力组合成一套能跑起来、能交付、能反馈、能持续改进的 agent 工作流。
```

一个好的 agent 不只是能聊天。它应该能用尽量少的 token 获取必要上下文，理解任务边界，执行受控动作，交付可验证结果，在不确定时反馈阻塞，并把重复的人类介入逐步沉淀成规则、prompt、skill、task template 或项目流程。

因此，Multigent 的产品重点应该从“创建对象”升级为“帮助用户跑通交付链路”。

## 目标用户旅程

### 首次管理员

理想路径：

```text
注册 / 登录
-> 创建 workspace
-> 选择团队模板或手动创建团队
-> 创建项目
-> 选择项目流程模板
-> 回答几个关键流程问题
-> 创建 agent
-> 配置模型账号、技能、连接、权限
-> 自动生成 project prompt、wakeup prompt、schedule、task templates
-> 下达第一条任务
-> 查看 run、日志、输出和阻塞
-> 调整 prompt / skill / 权限 / 模板
```

首次体验不能要求用户先理解所有概念。系统应该把复杂度拆成几个问题：

- 这个项目属于什么类型？
- 你用哪些工具？
- 大需求需要先评审方案吗？
- 谁负责审核？
- agent 可以直接改代码吗？
- 做完以后应该通知谁、怎么验收？

用户回答完这些问题，Multigent 应该生成一个可运行的初始流程。

### 普通成员 / Agent 负责人

普通成员不应该面对 workspace 管理控制台。他需要看到：

- 我负责的 agent。
- 这个 agent 现在能不能跑。
- 它最近做了什么。
- 哪些 run 失败了。
- 它缺什么上下文、权限或技能。
- 我怎么调教它，让下次少问我。

这里的用户不是“每一步调用 agent 的人”，而是 agent 的负责人和流程调优者。

## Prompt 与模板分层

Multigent 应该明确区分四层，不要把所有规则塞到一个 prompt 里。

### 1. Role Prompt

Role Prompt 描述职业能力、行为边界和通用质量准则。

适合放：

- 后端开发要关注高内聚、低耦合、可测试性、错误处理和可观测性。
- 前端开发要关注可访问性、响应式、状态管理和交互一致性。
- QA 要关注复现步骤、边界条件、回归风险和验收记录。
- PM 要先定义问题，再定义方案，并明确 non-goals。

不适合放：

- 某个企业的飞书审批流程。
- 某个项目的 GitLab 分支命名规则。
- 某个需求必须先写哪份内部文档。

Role Prompt 是“这个角色应该像什么样的专业人士”。

### 2. Team Prompt

Team Prompt 描述一个职能团队的协作原则。

适合放：

- 工程团队的代码质量标准。
- 产品、设计、开发、QA 之间如何交接。
- 何时升级风险。
- 什么情况需要人类确认。
- 团队通用产出格式。

Team Prompt 是“这个职能团队如何协作”。

### 3. Project Prompt

Project Prompt 是企业流程和项目上下文的核心承载层。

适合放：

- 项目背景、目标、关键约束。
- 当前项目使用 GitHub、GitLab、飞书、Linear、ONES 还是其他系统。
- 大需求是否必须先写 spec / PRD / 技术方案。
- 小 bug 是否可以直接修。
- 开发前需要如何做计划。
- 开发后需要如何自测、联调、发给 QA。
- 谁有权审批方案、发版、外部发布。
- 项目文档、接口定义、测试环境、部署流程在哪里。

Project Prompt 是“这个项目在这家公司里怎么推进”。

这也是小白用户最需要 Multigent 帮忙生成和打磨的部分。

### 4. Task Prompt / Wakeup / Schedule

Task Prompt 是具体这一次要做什么。

Wakeup Prompt 和 Schedule 是持续运行入口：

- 检查是否有 pending task。
- 检查失败 run。
- 检查未回复消息。
- 检查阻塞项。
- 生成日报、周报或项目状态。
- 定期扫描 issue、PR、客户反馈或告警。

Wakeup Prompt 不应该重复项目流程全文。它应该引用项目规则，并明确周期性要做的动作。

## 模板系统设计

Multigent 至少需要四类模板。

### Team Template

用于快速创建一个职能团队和角色集合。

示例：

- 软件交付团队：PM、UI、Frontend、Backend、QA、Reviewer。
- 客服支持团队：Support、Triage、Knowledge Base Maintainer。
- 运营团队：Content、Community、Campaign、Data Analyst。

Team Template 输出：

- team name 建议。
- team prompt。
- roles。
- role prompts。
- 推荐 skills。

Team Template 不应该绑定到单个项目。一个模板可以创建多个团队，也可以被用户保存成自己的模板。

### Playbook Template / 协作方案模板

这是下一阶段最关键的模板。

它不是“复制一个项目”，也不只是生成一个 workflow，而是把某类业务协作方式打包成可安装、可运行、可调整的一整套结构。

示例：

- 软件研发项目。
- Bug 修复项目。
- 内部工具项目。
- 客服自动化项目。
- 内容生产项目。
- 数据分析项目。

Playbook Template 输出：

- project prompt 草稿。
- 推荐团队/角色组合。
- 推荐 role prompt。
- 推荐 skills。
- 推荐 workflow。
- 推荐 agent 列表。
- 推荐 wakeup prompt。
- 推荐 schedule。
- 推荐 task templates。
- 推荐连接类型。
- 需要用户补齐的问题清单。

软件研发项目模板可以包含：

```text
大需求：
1. 先写 problem statement 和方案。
2. 方案通过负责人审核后再开发。
3. 大需求拆成可验收子任务。
4. 每个子任务完成后记录变更、测试和风险。
5. 全部完成后触发 QA 验收。

小 bug：
1. 先复现或说明无法复现。
2. 定位根因。
3. 做最小修复。
4. 补测试或说明为什么不能补。
5. 总结影响范围和验证结果。
```

这里的流程应该允许用户配置工具差异：

- GitHub 或 GitLab。
- 飞书文档或 Notion。
- Linear、Jira、ONES 或内置 task。
- Slack、飞书、钉钉或只用 Web。

模板不能假设所有企业用同一套工具。

更完整的设计见 [playbook-template-design.md](playbook-template-design.md)。

### Task Template

Task Template 降低用户写 prompt 的成本。

示例：

- 修复 bug。
- 实现小需求。
- 写技术方案。
- 做代码 review。
- 做 QA 验收。
- 整理会议纪要。
- 总结失败 run。
- 把人工介入沉淀成 prompt/skill。

Task Template 应该包含：

- 标题模板。
- prompt 模板。
- 必填字段。
- 适用角色。
- 推荐验收标准。
- 是否需要人工审批。

### Agent Ready Template

Agent Ready Template 不是用户显式选择的模板，而是系统判断一个 agent 是否准备好接活的 checklist。

至少检查：

- 已绑定模型账号。
- 模型可用。
- 已设置负责人。
- 已继承 team / role / project prompt。
- 已绑定必要 skills。
- 已授权必要 connections。
- wakeup prompt 是否存在。
- scheduler 是否配置。
- 最近一次 test run 是否成功。

Agent detail 页面应该直接展示这些状态，而不是让用户自己猜为什么 agent 跑不起来。

## 推荐的产品入口

### 创建项目时

新增“选择协作方案”步骤：

```text
Create Project
-> Select Playbook
-> Answer Setup Questions
-> Generate Project Prompt / Wakeup / Schedule / Task Templates
-> Continue to Add Members / Agents
```

如果用户不想选模板，可以手动创建空项目。

### Agent 详情页

Agent 详情页应该聚焦三件事：

- 配置能力：模型、凭证、技能、连接。
- 配置行为：role prompt、project prompt、wakeup prompt、task rules。
- 查看表现：任务、run、失败原因、token、人工介入、下一步建议。

当 agent 表现不好时，页面应该帮助用户判断原因：

- 缺上下文。
- 缺权限。
- 缺工具。
- prompt 太泛。
- 任务太大。
- 模型不合适。
- 没有验收标准。

然后提供改进动作：

- 更新 project prompt。
- 新增 skill。
- 新增 task template。
- 调整 schedule。
- 授权 connection。
- 拆分任务。

### Run 失败后

失败 run 不应该只是日志。

系统应该生成一份结构化诊断：

```text
失败类型：
- 权限不足 / 工具不可用 / 上下文缺失 / 测试失败 / 模型输出不可用 / 人工审批缺失

建议动作：
- 补充某个文档到项目知识库。
- 给 agent 授权某个连接。
- 把本次人工说明沉淀到 project prompt。
- 把重复操作沉淀为 skill。
- 拆成更小任务重试。
```

这才是从“工具”变成“agent 运营平台”的关键。

## 竞品/参考项目调研

### CrewAI

CrewAI 的核心抽象是 Agent、Task、Crew、Flow。

它做得好的地方：

- Agent 明确有 role、goal、backstory、tools、memory、delegation 等属性。
- Task 明确有 description、expected_output、agent、tools 等属性。
- Crew 支持 sequential 和 hierarchical 执行。
- Flow 提供事件驱动、状态管理、条件分支、循环和更强控制流。
- Human feedback 支持 approve/reject/revise 这类人工审核和修订循环。
- 它区分 tools/actions 与 skills/knowledge/context，这个分层值得参考。

它没有直接解决的问题：

- 它主要是开发者框架，不是面向普通企业用户的 SaaS 工作流产品。
- 企业内部流程、工具差异、项目 playbook 需要开发者自己编码。
- 它能表达流程，但不会主动引导小白用户把企业流程变成 project prompt / wakeup / task template。
- 对“agent 长期作为公司成员被运营、调教、审计”的产品体验支持有限。

对 Multigent 的启发：

- 借鉴 Agent/Task 的结构化字段。
- 借鉴 Flow 的事件驱动和人工反馈循环。
- 不照搬“代码定义流程”的使用方式。Multigent 应该把这些能力产品化成 Web 向导和模板。

### MetaGPT

MetaGPT 的核心思想是 `Code = SOP(Team)`，把软件公司的 SOP 物化到多角色 agent 团队里。

它做得好的地方：

- 明确把 PM、Architect、Project Manager、Engineer、QA 等角色串成软件开发流水线。
- 强调 SOP、结构化中间产物、文档和图，而不是让 agent 随意聊天。
- 使用共享消息池和订阅机制减少点对点传话。
- 强调可执行反馈：工程师写代码后运行测试，根据错误继续修复。
- 论文里也展示了增加角色和可执行反馈能降低人工修订成本、提升可执行性。

它没有直接解决的问题：

- 它更像“软件公司模拟器”，流程相对固定。
- 企业真实项目里的工具、审批、文档、权限、部署差异没有被产品化处理。
- 它适合从一句需求生成一个软件项目，不等同于长期运营公司里的多个 agent 同事。
- 缺少 SaaS 化的 workspace、成员权限、凭证隔离、审计、可视化调教闭环。

对 Multigent 的启发：

- Playbook Template 应该学习 MetaGPT：流程要靠 SOP 和结构化产物，而不是靠 agent 自由发挥。
- 大需求必须有中间产物：problem statement、spec、API design、task breakdown、test plan。
- Agent 之间的协作应该尽量通过结构化文档、task、run summary 和 shared context，而不是只靠聊天。

### Multica

Multica 的核心是“把 coding agent 放到任务管理平台里”，让 agent 像团队成员一样被分配 issue、执行任务、汇报阻塞、积累 skill。

它做得好的地方：

- 用户心智简单：创建 agent，把 issue 分配给 agent。
- Agent 是一等成员，能出现在看板、评论和任务生命周期里。
- Autopilot 可以做定时、Webhook、手动触发的周期性工作。
- Skill 作为可复用能力从数据库物化到 runtime 工作目录。
- Agent 快速创建计划中提出了模板、Skill Finder、AI Create Agent，说明他们也意识到“空白 instructions + 手动挑 skill”门槛太高。

它没有直接解决的问题：

- 它仍偏“issue/agent/runtime 管理”，项目流程 playbook 抽象不足。
- Agent template 主要是 instructions + skill，不足以覆盖项目级 SOP、wakeup、schedule、task template。
- 企业如何把内部流程、审批、文档和工具映射到 agent 日常工作，还需要用户自己完成。
- 本地 daemon/runtime 模型对我们当前“云端 sandbox agent 同事”的定位不是最佳答案。

对 Multigent 的启发：

- 任务管理入口要足够直观。
- Skill 从数据库到 runtime 的物化机制值得借鉴。
- 但 Multigent 应该进一步做项目流程模板和 agent 调教闭环，而不是只做 agent 创建模板。

## Multigent 的差异化方向

Multigent 不应该做成 CrewAI 的低代码版，也不应该做成 MetaGPT 的软件公司流水线复刻，更不应该只是 Multica 的另一个 UI。

Multigent 应该定位为：

```text
面向企业团队的 agent workforce operations platform。
```

核心差异：

- 用 workspace / project / team / role / agent / task / run / audit 管理真实组织。
- 用 Playbook Template 帮企业把流程变成 agent 可执行上下文。
- 用 Agent Ready Checklist 降低首次跑通难度。
- 用 Run Diagnosis 和 Human Intervention Ledger 把失败和人工介入变成改进素材。
- 用 skill、credential、connection、sandbox、RBAC 保证 agent 能做事但不能越权。
- 用 Web、飞书/Lark 等日常入口让人自然介入 agent 的真实 session。

## 近期实现建议

### Phase 1：文档和模型先定

- 定义 Playbook Template 数据结构。
- 定义 Task Template 数据结构。
- 定义 Agent Ready Checklist 聚合逻辑。
- 更新现有 Team Template prompt，使 role prompt 更偏职业准则，而不是项目流程。

### Phase 2：项目创建向导

- 创建项目时可选 playbook。
- 根据 playbook 问 5-8 个关键问题。
- 生成 project prompt、wakeup prompt、默认 schedule 和 task templates。
- 创建后进入项目首页 checklist。

### Phase 3：Agent Ready Checklist

- Agent detail 顶部展示 ready 状态。
- 缺配置时给出明确下一步。
- 支持一键 test run。

### Phase 4：任务模板

- 新建 task 支持选择模板。
- 模板根据 project playbook 和 agent role 自动填充 prompt 骨架。
- 支持用户把常用 task 保存为 workspace/project 模板。

### Phase 5：失败诊断和沉淀

- Run 失败后生成 failure classification。
- 支持从 run summary 一键创建：
  - prompt amendment
  - skill draft
  - task template
  - doc entry
  - follow-up task

## 最小可验收闭环

第一版不需要解决所有行业，只需要让软件研发项目跑通：

```text
创建 workspace
-> 选择软件交付团队模板
-> 创建项目并选择软件研发 playbook
-> 回答 Git 平台、文档系统、审核人、需求大小规则
-> 创建 PM / Dev / QA agent
-> 系统生成 project prompt / wakeup / task templates
-> 下达一个 bug 修复任务
-> agent 完成或反馈阻塞
-> 用户查看 run 并调教
-> 重试后成功
```

这条链路跑顺后，再扩展到客服、运营、数据分析等场景。

## Sources

- CrewAI documentation, Agents / Tasks / Flows / Human Feedback: https://docs.crewai.com/llms-full.txt
- CrewAI repository overview: https://github.com/crewaiinc/crewai
- MetaGPT repository: https://github.com/foundationagents/metagpt
- MetaGPT paper: https://arxiv.org/html/2308.00352v6
- Local Multica repository reviewed at `/root/code/spaceship/3rd/multica`
