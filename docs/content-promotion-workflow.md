# 内容宣传协作流程

> Status: Draft  
> Scope: 文章、SEO 内容、产品宣传文案、社媒分发  
> Related: `collaboration-workflow-state-machine.md`, `industry-playbooks-and-remote-registry.md`, `product-launch-and-growth-todo.md`

本文档设计 Multigent 的“文章宣传流程”。它不是一个纯写稿流程，而是一条面向 agent 协作的内容生产状态机：

```text
主题输入
-> 研究与定位
-> 大纲
-> 人类审核
-> 全文草稿
-> 编辑与事实核查
-> 素材 brief
-> 素材制作
-> 最终组装
-> 发布审批
-> 发布与分发
-> 数据复盘
```

核心目标不是让 agent 一次性写出一篇文章，而是让内容生产从“人反复催稿、改稿、找素材、发平台”变成：

```text
Agent 主动推进产物。
Human 只在关键判断点 review。
每一步都有结构化输入输出。
最终沉淀成可复用的内容生产 playbook。
```

## 1. 为什么需要这条流程

网上对 AI agent 内容生产的共识基本一致：agent 可以承担 research、outline、draft、SEO review、image creation、CMS publishing、social distribution 等工作，但仍需要人类在品牌判断、事实核查、原创观点、合规和最终发布上把关。

这说明内容宣传是 Multigent 很适合切入的非研发场景：

- 它有多角色协作：策略、写作、编辑、视觉、发布、数据复盘。
- 它有明确产物：brief、大纲、全文、素材清单、最终稿、发布链接、复盘报告。
- 它天然需要 human-in-the-loop：大纲、观点、事实、品牌口吻和外部发布都不能完全放飞。
- 它容易量化：耗时、返工次数、token、发布数量、收录、阅读、转化、线索。

## 2. 角色设计

第一版建议用这些角色，不要拆得太细：

| 角色 | 类型 | 负责什么 |
| --- | --- | --- |
| Content Strategist | Agent | 主题理解、受众定位、关键词方向、传播角度 |
| Research Agent | Agent | 收集资料、竞品内容、引用来源、事实依据 |
| Writer Agent | Agent | 大纲和全文草稿 |
| Editor Agent | Agent | 结构、可读性、品牌语气、SEO、事实核查 |
| Visual Agent | Agent | 图片需求、配图 brief、封面图、插图、社媒图 |
| Publisher Agent | Agent | 平台格式适配、发布草稿、分发文案 |
| Human Owner / Editor | Human | 审核主题、大纲、全文、素材和最终发布 |

后续可以按客户实际情况扩展：

- Legal / Compliance Reviewer。
- Founder Voice Reviewer。
- Customer Success Reviewer。
- SEO Specialist。
- Community Operator。

但第一版不要一上来过度复杂。

## 3. 推荐流程

### 3.1 内容需求输入

由人类创建任务，输入主题范围、目的和发表平台。

输入字段：

| 字段 | 说明 |
| --- | --- |
| `topic` | 文章主题或宣传方向 |
| `goal` | 目的：SEO、品牌认知、产品发布、客户教育、融资叙事、社媒传播等 |
| `audience` | 目标读者 |
| `platforms` | 期望发布平台，例如官网博客、公众号、知乎、Twitter/X、LinkedIn、飞书文档 |
| `source_doc_ids` | 参考资料 docID 列表 |
| `constraints` | 不能说什么、必须提什么、品牌语气、敏感边界 |

输出字段：

| 字段 | 说明 |
| --- | --- |
| `content_brief_doc_id` | 内容 brief 文档 docID |
| `summary` | 本次内容任务的一句话目标 |

### 3.2 研究与定位

Research Agent / Content Strategist 读取资料，补充搜索和竞品分析。

它不直接写正文，而是回答：

- 读者真正关心什么。
- 这篇文章要占领哪个关键词或认知位置。
- 竞品或同类文章已经说了什么。
- 我们有什么不同角度、证据或独特观点。
- 哪些事实必须核查。

输出字段：

| 字段 | 说明 |
| --- | --- |
| `research_doc_id` | 研究资料与来源文档 docID |
| `seo_brief_doc_id` | SEO brief docID，包括关键词、搜索意图、标题候选、meta 描述候选 |
| `angle_recommendation` | 推荐切入角度 |
| `risk_notes` | 事实、合规、品牌表达风险 |

### 3.3 大纲生成

Writer Agent 根据 brief 和 research 生成文章大纲。

大纲不要只是标题列表，必须包含：

- 标题候选。
- 文章核心论点。
- 每一节解决什么问题。
- 每一节需要哪些证据或素材。
- 预计需要哪些图片或表格。
- SEO 要覆盖的关键词和相关问题。
- 不写什么。

输出字段：

| 字段 | 说明 |
| --- | --- |
| `outline_doc_id` | 大纲文档 docID |
| `title_options` | 标题候选 |
| `asset_placeholders` | 初步素材占位清单 |
| `open_questions` | 需要人类确认的问题 |

### 3.4 大纲人工审核

Human Editor 审核大纲。这里是第一道关键 gate。

审核输出字段：

| 字段 | 说明 |
| --- | --- |
| `decision` | `approve`、`request_changes`、`cancel` |
| `comments` | 审核意见。打回时必须写清楚要改什么 |
| `approved_outline_doc_id` | 通过后的最终大纲 docID |

流转规则：

```text
approve -> 全文草稿
request_changes -> 大纲生成
cancel -> 结束
```

### 3.5 全文草稿

Writer Agent 写完整正文。素材未完成时，用结构化占位符，不要随意编造图片。

正文要求：

- 先按 approved outline 写。
- 大段内容必须写入知识库文档。
- 图片、截图、图表用明确描述占位，例如 `[IMAGE: 产品工作台截图，突出 workflow board 和 agent handoff]`。
- 涉及事实、数字、案例必须标注来源或标为待核查。
- 不允许虚构客户案例、收入数字、合作伙伴和权威背书。

输出字段：

| 字段 | 说明 |
| --- | --- |
| `draft_doc_id` | 全文草稿 docID |
| `asset_brief_doc_id` | 图片与素材需求 brief docID |
| `fact_check_list_doc_id` | 待核查事实清单 docID |
| `summary` | 草稿摘要 |

### 3.6 编辑与事实核查

Editor Agent 对草稿做质量检查，不直接进入发布。

检查维度：

- 是否符合 brief 和大纲。
- 是否有真实观点，而不是泛泛而谈。
- SEO 标题、H1/H2、meta、关键词覆盖是否自然。
- 事实、引用、数据是否可追溯。
- 品牌语气是否一致。
- 是否存在过度承诺、敏感表述或外部发布风险。
- CTA 是否明确。

输出字段：

| 字段 | 说明 |
| --- | --- |
| `editor_review_doc_id` | 编辑审核报告 docID |
| `revised_draft_doc_id` | 编辑后的草稿 docID |
| `quality_score` | 1-5 分，低于 4 分建议打回 |
| `blocking_issues` | 阻塞问题列表 |

流转规则：

```text
quality_score >= 4 且无 blocking_issues -> 文案人工审核
否则 -> 全文草稿
```

### 3.7 文案人工审核

Human Editor 审核完整文案。

审核输出字段：

| 字段 | 说明 |
| --- | --- |
| `decision` | `approve`、`request_changes`、`hold`、`cancel` |
| `comments` | 审核意见 |
| `approved_copy_doc_id` | 通过后的最终文案 docID |

流转规则：

```text
approve -> 素材制作
request_changes -> 全文草稿
hold -> 暂停等待更多输入
cancel -> 结束
```

### 3.8 素材制作

Visual Agent 根据 `asset_brief_doc_id` 制作素材。

第一版不要求直接生成所有图片，也可以输出清晰的素材包：

- 封面图 prompt。
- 插图 prompt。
- 截图清单。
- 图表数据。
- 社媒卡片文案。
- 需要人工提供的产品截图或品牌素材。

输出字段：

| 字段 | 说明 |
| --- | --- |
| `asset_manifest_doc_id` | 素材清单 docID |
| `generated_assets_doc_id` | 已生成素材 docID 或文件引用 |
| `missing_assets` | 仍需人工补充的素材 |

### 3.9 素材人工审核

Human Editor / Designer 审核素材。

审核输出字段：

| 字段 | 说明 |
| --- | --- |
| `decision` | `approve`、`request_changes`、`skip_assets` |
| `comments` | 审核意见 |
| `approved_assets_doc_id` | 通过后的素材包 docID |

流转规则：

```text
approve -> 最终组装
request_changes -> 素材制作
skip_assets -> 最终组装
```

### 3.10 最终组装

Publisher Agent 把文案和素材组装成目标平台版本。

不同平台需要不同输出：

| 平台 | 组装重点 |
| --- | --- |
| 官网博客 | Markdown / HTML、slug、meta title、meta description、OG 图 |
| 公众号 | 标题、摘要、封面、正文排版、图片插入说明 |
| 知乎 | 标题、开头钩子、段落节奏、结尾引导 |
| Twitter/X | thread、单条文案、配图 |
| LinkedIn | professional tone、摘要、CTA |
| 飞书文档 | 内部评审版、发布 checklist |

输出字段：

| 字段 | 说明 |
| --- | --- |
| `final_package_doc_id` | 最终发布包 docID |
| `platform_versions_doc_id` | 各平台版本 docID |
| `publish_checklist_doc_id` | 发布检查清单 docID |

### 3.11 最终发布审批

Human Owner 做最终发布审批。

这里必须是 human gate，因为外部发布存在品牌、承诺和舆论风险。

审核输出字段：

| 字段 | 说明 |
| --- | --- |
| `decision` | `publish`、`request_changes`、`hold`、`cancel` |
| `comments` | 审批意见 |
| `approved_package_doc_id` | 最终审批通过的发布包 docID |

流转规则：

```text
publish -> 发布与分发
request_changes -> 最终组装
hold -> 暂停
cancel -> 结束
```

### 3.12 发布与分发

Publisher Agent 执行发布或创建待发布草稿。

第一版建议不要默认自动外发，而是按平台风险分级：

| 风险 | 动作 |
| --- | --- |
| 低风险内部渠道 | 可自动发布 |
| 官网草稿 / CMS draft | 可自动创建草稿 |
| 公众号 / 社媒 / 邮件外发 | 默认创建草稿，由人点击发布 |
| 涉及价格、客户、融资、承诺 | 必须 human 最终确认 |

输出字段：

| 字段 | 说明 |
| --- | --- |
| `published_urls` | 已发布链接 |
| `draft_urls` | 已创建草稿链接 |
| `distribution_doc_id` | 分发记录 docID |
| `failed_channels` | 发布失败的平台和原因 |

### 3.13 数据复盘

Analytics Agent 在发布后定时复盘。

它不属于主发布链路，但应该作为后续定时 workflow 或 wakeup routine。

输出字段：

| 字段 | 说明 |
| --- | --- |
| `performance_report_doc_id` | 数据复盘报告 docID |
| `metrics` | PV、阅读、点击、收录、排名、转化、评论等 |
| `optimization_suggestions` | 标题、摘要、内链、二次分发、更新建议 |
| `next_content_ideas` | 下一批选题 |

## 4. 第一版流程图

```text
Content Intake
  -> Research & SEO Brief
  -> Outline Draft
  -> Human Outline Review
       approve -> Full Draft
       request_changes -> Outline Draft
       cancel -> End
  -> Editorial / Fact Check
       pass -> Human Copy Review
       fail -> Full Draft
  -> Human Copy Review
       approve -> Asset Production
       request_changes -> Full Draft
       hold -> Hold
       cancel -> End
  -> Asset Production
  -> Human Asset Review
       approve -> Final Assembly
       request_changes -> Asset Production
       skip_assets -> Final Assembly
  -> Final Assembly
  -> Human Publish Approval
       publish -> Publish & Distribute
       request_changes -> Final Assembly
       hold -> Hold
       cancel -> End
  -> Publish & Distribute
  -> Post-publish Review
  -> End
```

## 5. 任务模板建议

这条流程应该配 3 个项目级任务模板。

### 5.1 标准文章宣传任务

适合官网博客、公众号、知乎、长文。

默认绑定完整流程：

```text
intake -> research -> outline -> draft -> review -> assets -> publish
```

### 5.2 快速社媒宣传任务

适合 Twitter/X、LinkedIn、朋友圈、社群短内容。

简化流程：

```text
intake -> angle -> short copy -> human review -> publish draft
```

### 5.3 SEO 内容更新任务

适合旧文章刷新、排名优化。

流程：

```text
existing content audit
-> keyword gap
-> update draft
-> human review
-> update publish
-> performance monitor
```

## 6. 与 Wakeup 的关系

Workflow 管一次具体内容任务。

Wakeup routine 管持续发现机会，例如：

- 每天查看关键词排名变化。
- 每周找 5 个值得写的主题。
- 监控竞品新文章。
- 检查已发布文章是否收录。
- 检查社媒评论和潜在线索。
- 发现机会后用任务模板创建新的内容任务。

也就是说：

```text
Wakeup = 发现机会、维护队列、复盘数据。
Workflow = 某一篇文章从想法到发布的执行流转。
```

不要把“每日找选题”硬塞进单篇文章 workflow。那应该是 Content Strategist 的 routine。

## 7. 需要的外部工具

第一版建议支持：

| 工具 | 用途 |
| --- | --- |
| Web Search / Exa / Brave | research、竞品文章、事实核查 |
| Google Search Console | 收录、关键词、CTR |
| Google Analytics / Plausible | 阅读和转化 |
| CMS / GitHub / GitLab | 官网内容发布 |
| Feishu / Lark Docs | 草稿、审核、内部知识库 |
| Notion / Confluence | 客户知识库资料 |
| Figma / Image generation | 素材与视觉 |
| Twitter/X / LinkedIn | 分发草稿或发布 |

发布类工具默认应该走“创建草稿 + 人类最终确认”，不要第一版就全自动外发。

## 8. 质量标准

一篇内容进入发布前，至少满足：

- 主题与目标读者清楚。
- 大纲经过人类确认。
- 正文有明确观点，不只是泛泛总结。
- 事实和数据可追溯。
- 图片或素材不是随意凑数。
- 平台格式适配完成。
- 外部发布经过最终审批。
- 所有关键产物都沉淀为 docID。

## 9. 评估指标

流程级指标：

- 从主题输入到最终发布的总耗时。
- Human review 次数。
- 每个节点返工次数。
- 每个节点 token 消耗。
- 每个节点等待人类审核的时间。
- 每篇文章平均成本。

内容效果指标：

- 发布数量。
- 收录数量。
- 搜索曝光。
- CTR。
- 阅读完成率。
- 评论、收藏、转发。
- 线索或注册转化。
- 发布后 7 天 / 30 天更新次数。

真正要优化的是：

```text
用更少的人类同步沟通和更少的 token，稳定产出更可发布、更能转化的内容。
```

## 10. Multigent 落地建议

第一版可以做成一个 `Content Promotion` playbook，包含：

- Team: `content`
- Roles:
  - content-strategist
  - researcher
  - writer
  - editor
  - visual-producer
  - publisher
- Skills:
  - content-briefing
  - seo-research
  - outline-review
  - editorial-fact-check
  - visual-brief
  - platform-publishing
  - post-publish-analysis
- Workflow:
  - `Content Promotion Workflow`
- Task Templates:
  - `standard-content-article`
  - `quick-social-post`
  - `seo-refresh`

不要把它只命名为 SEO workflow。SEO 只是目标之一，企业更容易理解的是“内容宣传”或“内容发布协作”。

## 11. 开放问题

- 素材制作第一版是否真的接图像生成，还是先只输出素材 brief？
- 发布动作第一版是否只创建草稿，不直接 publish？
- 文章最终稿是否应该支持 Markdown、HTML、飞书文档、公众号格式多种 artifact？
- 内容审核是否需要合规节点，还是由客户自己在流程里添加？
- 是否需要把 Founder Voice / Brand Voice 作为独立 skill？
- 性能数据复盘是否作为 workflow 后置节点，还是作为独立 wakeup routine？

## 12. 建议下一步

1. 先把这条流程做成一个内置 workflow template。
2. 再做一个 `Content Promotion` playbook，包含角色、skills 和任务模板。
3. 第一版发布动作只创建草稿或发布包，不自动外发。
4. Product Tour 里增加一个非研发 example：让三个 agent 协作完成一篇短内容从选题到发布包。
