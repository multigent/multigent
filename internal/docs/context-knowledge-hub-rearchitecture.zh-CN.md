# 知识库 / Context Hub 重构方案

日期：2026-08-29

## 1. 背景

Multigent 2.x 已经把 Agent 从“项目里的成员”逐步升级为 workspace 级协作主体。接下来真正影响协作质量的，不只是任务、流程和 IM，而是：

- Agent 能拿到什么信息。
- 哪些信息应该长期沉淀。
- 哪些信息只作为注意力提醒。
- 哪些信息只在权限允许时按需读取。
- 人类和 Agent 如何围绕同一套知识资产协作，而不是各自维护一份碎片化资料。

当前产品里同时存在“知识库”“上下文库”“文件”“抓取器”“信号”等概念，已经开始让用户难以区分：

- 什么时候把内容放知识库。
- 什么时候放上下文。
- 文件和文档算不算知识。
- 抓取器抓到的数据去哪儿。
- agent 什么时候该主动看，什么时候只需要被提醒。

因此需要一次结构性的重构。

## 2. 核心判断

### 2.1 知识库应该是总入口

我们应该把“知识库”提升为总入口，而不是把“知识库”和“上下文”做成两个并列大入口。

更准确地说：

- **知识库**：人类管理和组织信息资产的总入口。
- **Context**：Agent 在运行时看到的消费视角。
- **Signal**：提醒 Agent 去看某些上下文，但不直接把所有内容塞进 prompt。

### 2.2 所有内容本质上都是资产

文件、文档、会话、会议纪要、抓取数据、技能、规则、笔记，本质上都只是不同格式的资产。

它们可以共享同一套：

- 元数据
- 权限
- 标签
- 来源
- 版本
- 关系
- 检索
- 可见性
- 审计

### 2.3 不做全量注入

我们不采纳“把所有信息都塞给 agent”的思路。

原因：

- 上下文窗口有限。
- 噪音会压过信号。
- 权限边界会混乱。
- agent 会失去自主判断。

更合理的是：

- 通过 Signal 先提示“这可能重要”。
- Agent 再决定是否读取。
- 只把必要摘要和必要引用注入到当前任务上下文。

### 2.4 agent 的主体性要保留

agent 不是工具触发器，而是协作对象。

所以系统要支持：

- agent 自己判断是否需要看资料。
- agent 自己决定是否需要向人类确认。
- agent 自己决定是否把当前信息沉淀成资产或知识。
- agent 自己决定是否把某条 signal 标记为已读、忽略或后续处理。

## 3. 参考项目的启发

### 3.1 OpenViking

OpenViking 的核心启发是：

- 它把 context 做成一个**可导航的文件系统**。
- 它把 memory / resource / skill 分层管理。
- 它强调**按需加载**，不是全量注入。
- 它有较清晰的**抽取流水线**：解析、结构化、语义化、索引分离。

对我们的启发：

- context 不应该是黑盒向量库。
- 内容应该有可浏览、可追踪、可复用的组织结构。
- 抓取、解析、语义、索引应该解耦。

### 3.2 TencentDB-Agent-Memory

TencentDB-Agent-Memory 的核心启发是：

- 它把 conversations / docs / code 统一成可复用的 memory assets。
- 它强调团队级 memory hub，而不是单 agent 私有记忆。
- 它有治理、共享、ACL、版本、绑定关系等能力。

对我们的启发：

- 记忆不是某个 agent 独占。
- 资产应该能跨 agent 复用。
- 需要治理层，而不是单纯存文本。
- 资产要能被“装备”给不同 agent，而不是直接硬编码进 prompt。

### 3.3 我们和它们的差异

我们不想走“所有内容都直接注入”的路线。

我们的差异点是：

- 保留 Attention Signal。
- 保留任务 / 流程 / 审计。
- 保留 agent 主体性。
- 保留权限控制。
- 让 agent 自己决定看什么、何时看、看多少。

## 4. 目标

### 4.1 产品目标

把以下入口统一起来：

- 文件
- 文档
- 会话记录
- 会议纪要
- 抓取器数据
- 结构化知识
- 技能/规则

让用户只理解一件事：

> 我在给 agent 准备可用的信息资产。

### 4.2 Agent 目标

让 agent 具备：

- 可信信息源的感知能力。
- 按权限检索的能力。
- 任务驱动的主动阅读能力。
- 基于信号的自我唤醒和主动跟进能力。

### 4.3 工程目标

把数据底座从“功能散点”升级为“统一资产平台”，便于：

- 收集
- 清洗
- 去重
- 标记敏感度
- 构建索引
- 权限控制
- 记录审计
- 支持后续扩展

## 5. 非目标

第一版不做：

- 向后兼容旧知识库 / 上下文模型。
- 复杂知识图谱编辑器。
- 强制全量向 agent 注入全部资产。
- 过早把 signal 和 IM 强耦合。
- 一次性把所有 collector 都做齐。
- 把“文件/知识库/上下文”并列成多个用户必须理解的主入口。

## 6. 新的抽象

### 6.1 Knowledge Hub

统一的知识资产中心。

用户看到的是：

- 资产
- 视图
- 抓取器
- 信号
- 权限

### 6.2 Asset

所有内容统一抽象成 Asset。

Asset 可能是：

- file
- doc
- chat_memory
- meeting_note
- code_snippet
- collector_item
- skill
- policy
- summary

每个 Asset 都应该有：

- id
- type
- title
- source
- owner
- visibility
- scope
- tags
- sensitivity
- created_at
- updated_at
- version
- relations

### 6.3 View

View 是给人看的组织方式，不改变底层存储。

建议至少支持：

- 目录视图
- 标签视图
- 项目视图
- Agent 视图
- 来源视图
- 时间视图

后续可以扩展到：

- 图谱视图
- 责任人视图
- 流程视图

### 6.4 Collector

Collector 是外部或独立进程，负责把外部信息抓进来。

它不属于 agent runtime 本身，也不应该和 agent 执行环境强耦合。

Collector 负责：

- 读取来源
- 过滤增量
- 标准化
- 去重
- 脱敏
- 生成资产
- 上报状态

### 6.5 Signal

Signal 是注意力层，不是存储层。

它回答的是：

- 哪些内容发生变化。
- 哪些内容值得 agent 关注。
- 哪个 agent 应该收到提醒。
- 这条提醒是高优先级还是低优先级。

Signal 不等于任务，不等于知识，不等于文档。

## 7. 前端信息架构

### 7.1 一级导航

建议一级导航只保留一个统一入口：

- **知识库**

不要再让用户同时看到“知识库”“上下文库”“文件库”三个并列入口。

### 7.2 知识库内部结构

建议页面内部拆成这些 tab：

1. 资产
2. 抓取器
3. 视图
4. 信号
5. 权限

### 7.3 页面职责

#### 资产页

展示全部资产，包括：

- 文件
- 文档
- 会话
- 抓取内容
- 技能/规则

支持：

- 搜索
- 过滤
- 预览
- 打标签
- 查看来源
- 查看关系

#### 抓取器页

展示和配置 collector：

- 连接什么来源
- 抓取什么范围
- 是全量还是增量
- 抓取频率
- 最近运行状态
- 失败原因

#### 视图页

展示不同组织方式：

- 目录
- 标签
- 项目
- Agent
- 来源

#### 信号页

展示 attention signals：

- 由谁发出
- 来自哪里
- 给谁看
- 优先级
- 已读 / 未读 / 已处理
- 是否转成任务

#### 权限页

展示：

- 谁能看
- 谁能改
- 谁能导出
- 谁能作为 Agent 使用

### 7.4 agent 侧展示

Agent 不需要看到“知识库”和“上下文库”的产品争论。

Agent 只需要看到：

- 可读资产
- 关联资产
- 当前信号
- 可用检索工具
- 权限范围

## 8. 后端逻辑

### 8.1 服务拆分

建议后端拆成四块：

1. **Asset Service**
   - 资产 CRUD
   - 版本管理
   - 元数据

2. **Collector Service**
   - 抓取器注册
   - 抓取任务
   - 同步状态
   - 失败重试

3. **Retrieval Service**
   - 检索
   - 分层摘要
   - 按需加载
   - 权限过滤

4. **Signal Service**
   - 信号注册
   - 信号分发
   - 信号 ack
   - 信号归档

### 8.2 处理链路

建议链路如下：

```text
Source
  -> Collector
  -> Normalize / Dedupe / Classify
  -> Asset Store
  -> Index / Summary
  -> Optional Signal
  -> Agent retrieves on demand
```

### 8.3 读取策略

agent 在运行时不直接拿到所有资产，而是：

- 先拿 signal
- 再根据 signal 读摘要
- 再按需展开原文
- 再按需继续追索关系资产

### 8.4 写入策略

人类、collector、agent 都可以产生资产，但写入规则不同：

- 人类可以直接上传文件 / 文档 / 备注。
- collector 可以批量写入结构化资产。
- agent 可以生成总结、手记、交接说明，但应带来源和审计。

## 9. 存储设计

### 9.1 基本表

建议至少有：

- `assets`
- `asset_contents`
- `asset_relations`
- `asset_views`
- `collectors`
- `collector_runs`
- `signals`
- `signal_recipients`
- `signal_acks`
- `asset_permissions`
- `asset_versions`

### 9.2 Asset 元数据

`assets` 记录：

- `id`
- `workspace_id`
- `type`
- `title`
- `source_type`
- `source_id`
- `source_item_id`
- `owner_type`
- `owner_id`
- `scope_type`
- `scope_id`
- `visibility`
- `sensitivity`
- `status`
- `tags`
- `dedupe_key`
- `created_at`
- `updated_at`

### 9.3 内容存储

`asset_contents` 记录：

- 原文
- 摘要
- 结构化字段
- 引用片段
- 存储路径 / blob 引用

### 9.4 关系存储

`asset_relations` 记录：

- 资产与资产的引用关系
- 资产与任务的关系
- 资产与 agent 的绑定关系
- 资产与项目 / 团队 / 角色的关系

### 9.5 权限存储

`asset_permissions` 记录：

- owner
- team
- project
- agent
- user
- role

权限规则要支持：

- 读
- 写
- 绑定
- 删除
- 导出

### 9.6 视图存储

`asset_views` 只是组织方式，不改变底层资产。

例如：

- 某个知识空间目录树
- 某个项目视图
- 某个标签集合

## 10. Collector 设计

### 10.1 Collector 是独立能力

Collector 应该是独立进程或外部应用，而不是强绑定在主服务里。

原因：

- 不同来源的抓取节奏不同。
- 不同集成的鉴权方式不同。
- 有些抓取器需要单独部署。
- 有些抓取器适合社区/插件市场化。

### 10.2 Collector 的职责

Collector 负责：

- 注册
- 鉴权
- 抓取
- 去重
- 标准化
- 上报
- 状态心跳
- 失败重试

### 10.3 Collector 的输入输出

输入：

- 来源连接
- 抓取范围
- 同步策略
- 权限范围

输出：

- 标准 Asset
- 可选 Signal
- 运行状态

### 10.4 Collector 市场

以后可以把 collector 做成一种可扩展市场：

- 飞书 / Lark collector
- GitHub collector
- 文档 collector
- 本地 session collector
- 会议记录 collector

## 11. Signal 设计

### 11.1 Signal 不等于内容

Signal 只是提醒，不是内容本身。

### 11.2 Signal 来源

可来自：

- IM 消息
- 群聊 @
- 文档更新
- issue / PR 更新
- 定时任务
- collector 结果
- 外部事件

### 11.3 Signal 与 Agent 的关系

agent 收到 signal 后可以：

- 直接忽略
- 标记已看
- 拉取相关资产
- 生成任务
- 回复人类
- 推进流程

### 11.4 Signal 生命周期

建议状态至少有：

- new
- seen
- handled
- ignored
- deferred
- expired

## 12. Agent 如何使用这些内容

### 12.1 启动时

agent 不应默认加载全部资产。

启动时应加载：

- 权限范围内的基础资产索引
- 当前任务相关 Spec
- 与当前项目有关的长期记忆摘要
- 当前未读 signal

### 12.2 执行中

agent 通过工具按需读取：

- `mga context list`
- `mga context search`
- `mga context read`
- `mga signal list`
- `mga signal ack`

### 12.3 agent 自主策略

agent 应自己判断：

- 哪些信号重要
- 哪些内容需要补看
- 哪些内容值得总结成资产
- 哪些需要通知人类

## 13. 与任务 / 流程 / IM 的关系

### 13.1 任务仍然存在

Context Hub 不取代任务系统。

任务解决的是：

- 谁负责
- 什么时候完成
- 如何验收
- 流程怎么走

### 13.2 流程仍然存在

流程解决的是：

- 什么时候需要 human gate
- 什么时候需要 QA
- 什么时候需要发布
- 什么时候可以继续自动推进

### 13.3 IM 只是 signal 的一种载体

IM 不是知识库，不是任务，不是流程。

它只是 signal 的一种具体来源。

## 14. 不做向后兼容的策略

这次重构建议直接按 2.x 新架构推进，不保留历史包袱。

### 14.1 为什么可以不兼容

因为当前用户量还不大，继续兼容旧结构会导致：

- 概念污染
- 数据结构膨胀
- 页面入口混乱
- 权限和索引规则不统一
- 后续 collector / signal / asset 扩展都被旧逻辑拖住

### 14.2 怎么迁移

建议不是“逐步兼容”，而是：

1. 备份旧数据。
2. 生成新资产模型。
3. 用迁移脚本批量导入。
4. 重建索引和摘要。
5. 切换前端入口。
6. 关闭旧入口。

当前实现采取的是“新表承载 + 启动时一次性搬迁”的方式：

- 旧表 `context_sources / context_items / context_subscriptions` 仍保留，作为迁移输入和短期兼容层。
- 新表改名为 `knowledge_base_sources / knowledge_base_items / knowledge_base_subscriptions`。
- 服务启动时如果发现旧表存在，会把旧数据 `INSERT OR IGNORE` 到新表。
- 前端和新 API 入口统一切到 `knowledge-base`，旧 `/api/v1/context/...` 已移除。
- 这样可以避免双写、避免复杂回滚分支，也能让旧数据通过迁移后直接进入 2.x 新结构。

### 14.3 旧数据处理原则

- 能转成 asset 的，转成 asset。
- 不能可靠转的，保留为 archive。
- 所有迁移动作要可审计。

## 15. 第一版落地范围

建议第一版只做这几项：

- 统一知识库总入口。
- 资产列表与详情页。
- collector 注册与状态展示。
- 资产级权限。
- signal 列表与 ack。
- agent runtime 的按需读取能力。
- 文件 / 文档 / session 的统一导入。

不建议第一版就做：

- 图谱编辑器
- 复杂多维拖拽建模
- 全自动知识抽取 UI
- 过度复杂的 schema builder

## 16. 推荐命名

为了减少混淆，我建议：

- 人类侧主入口：**知识库**
- 技术底座：**Context Hub**
- 运行时视角：**Context**
- 提醒层：**Signal**
- 抓取进程：**Collector**
- 可管理对象：**Asset**

## 17. 结论

我们要做的不是“再做一个知识库”，而是：

> 把所有信息资产统一起来，用可治理、可检索、可权限控制、可按需加载的方式，给 agent 一个真正可持续工作的上下文底座。

这会比单纯把内容塞进 prompt 更适合长周期协作，也更符合 Multigent 的方向：

- agent 有主体性
- 信息有治理
- 信号驱动注意力
- 任务和流程负责承诺与审计
