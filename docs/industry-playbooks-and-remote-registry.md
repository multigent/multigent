# Industry Playbooks and Remote Registry

> Status: Draft  
> Scope: non-software playbooks, OpenMontage research, playbook registry, built-in template distribution

本文档接在 `playbook-template-design.md` 和 `customer-onboarding-playbook-generation-and-agent-improvement.md` 之后，回答两个问题：

1. Multigent 的协作方案是否只能围绕研发流程。
2. 内置协作方案是否应该继续写死在代码里。

核心结论：

```text
Multigent 应该把协作方案从“代码内置模板”升级为“可版本化、可远程分发、可安装的行业协作资产”。

代码里只保留少量稳定 fallback 和 registry/installer 能力；
真正快速迭代的 playbook、skill、workflow、role prompt 应该放到独立仓库，以 manifest + bundle 方式发布。
```

## 1. 为什么要看非研发协作方案

Garry、Matt Pocock、OpenSpec 这些方案都偏软件开发或产品研发。它们对早期验证很有价值，但如果我们要向更多企业证明 Multigent 的通用性，只靠研发场景会让用户误以为：

- Multigent 是一个更复杂的 Jira / Linear。
- Multigent 只适合工程团队。
- Agent 只是在写代码和修 bug。

这不是我们的最终定位。

Multigent 应该表达的是：

```text
任何需要多人、多角色、多工具、多产物、多轮审核的协作流程，都可以被建模成 agent 可执行的 playbook。
```

因此我们需要主动寻找非研发领域的开源流程样板，把它们转成可安装协作方案。

## 2. 候选行业方向

### 2.1 视频制作 / 内容生产

代表项目：OpenMontage。

这是目前最值得优先研究的非研发样板，因为它具备完整的多阶段生产流程：

- 需求理解。
- 参考视频分析。
- 方案选择。
- 脚本。
- 分镜。
- 素材生成。
- 剪辑。
- 合成。
- 审核。
- 发布。

它天然适合 Multigent 的 workflow、role、skill、external tools、artifact、human review gate。

### 2.2 深度研究 / 市场情报

代表项目：

- GPT Researcher。
- Open Deep Research。
- Local Deep Researcher。

这类项目的通用流程通常是：

```text
研究问题澄清
-> 搜索计划
-> 多轮检索
-> 来源评估
-> 反思缺口
-> 补充搜索
-> 报告生成
-> 引用核查
-> 人工审核
```

它适合包装成：

- 市场调研协作方案。
- 竞品分析协作方案。
- 行业报告协作方案。
- 投资研究协作方案。
- 客户背景调查协作方案。

### 2.3 销售 / CRM / 客户成功

可参考来源：

- n8n sales automation templates。
- CRM 自动化模板。
- 邮件跟进、线索评分、客户分层、报价流转等工作流。

这类流程非常适合 agent，但风险更高，因为涉及外发消息和客户承诺。第一版应该设计成：

```text
agent 起草
-> human review
-> agent 更新 CRM / 生成跟进任务
```

而不是直接自动发送给客户。

### 2.4 运营 / 支持 / 内部办公

可参考来源：

- n8n AI automation templates。
- 邮件分流。
- 工单分类。
- 客服回复草稿。
- 会议纪要转任务。
- 周报汇总。

这些流程更接近企业日常运转，适合做低风险入门 playbook。

### 2.5 设计 / 创意评审

可以参考：

- Figma / design review 工作流。
- 品牌规范检查。
- 营销素材审查。
- 创意 brief -> 初稿 -> 设计审核 -> 修改 -> 交付。

这类流程的关键不是自动生成最终设计，而是把 review criteria、brand guideline、design rationale 结构化。

## 3. OpenMontage 研究

### 3.1 它真正有价值的地方

OpenMontage 不是简单的视频生成脚本。它的核心结构是：

```text
Agent reads pipeline manifest
-> reads stage director skill
-> discovers tools
-> produces canonical artifacts
-> self-reviews
-> writes checkpoints
-> asks human approval when required
```

这跟 Multigent 的方向非常接近。

它给我们的启发不是“直接做视频生成”，而是：

```text
一个复杂业务流程可以被拆成：pipeline manifest + stage skill + tool registry + artifact schema + checkpoint + human gate。
```

这正是协作方案应该具备的结构。

### 3.2 OpenMontage 的流程模型

OpenMontage 的核心 pipeline 可以抽象为：

```text
research / idea
-> proposal
-> script
-> scene_plan
-> assets
-> edit
-> compose
-> publish
```

它的 canonical artifacts 包括：

| Artifact | 含义 |
| --- | --- |
| `brief` / `research_brief` | 需求、参考、受众、角度 |
| `proposal_packet` | 候选方案、工具路径、成本估计 |
| `script` | 脚本、旁白、节奏 |
| `scene_plan` | 分镜、视觉安排、素材需求 |
| `asset_manifest` | 图片、视频、音频、字幕等素材清单 |
| `edit_decisions` | 时间线、剪辑、字幕、音乐、转场 |
| `render_report` | 渲染结果、技术校验、问题 |
| `publish_log` | 发布平台、文案、链接、版本 |

这些 artifacts 很适合映射成 Multigent workflow 节点的结构化输入输出。

### 3.3 它的 human gate 设计

OpenMontage 很强调 human approval：

- 提案阶段需要用户确认工具路径、创意方向和预算。
- 脚本、分镜、素材阶段可以配置人工审核。
- 重大 provider/model/runtime 切换必须让用户确认。
- checkpoint 是流程恢复和审计的基础。

这对 Multigent 很重要，因为我们一直强调：

```text
人不应该成为同步阻塞的调用者，而应该成为异步审核者和流程调教者。
```

OpenMontage 的 gate 模型可以直接帮助我们完善 workflow：

- 节点完成后进入 `awaiting_review`。
- reviewer 可以 approve / request changes / abort。
- request changes 会带着结构化意见回到上游节点。
- 所有版本进入运行历史。

### 3.4 Multigent 可以如何包装 OpenMontage-style Playbook

建议做一个候选协作方案：

```text
Video Production Studio Playbook
```

中文名可以是：

```text
视频制作工作室
```

包含角色：

| Role | 责任 |
| --- | --- |
| Producer | 明确目标、受众、预算、交付形式 |
| Creative Director | 把 brief 转成创意方案和风格方向 |
| Script Writer | 输出脚本、节奏、旁白 |
| Scene Planner | 输出分镜和素材需求 |
| Asset Director | 生成或收集素材，记录成本和授权 |
| Editor | 输出剪辑决策、字幕、音乐、节奏 |
| Renderer / Composer | 负责合成、技术校验、最终交付 |
| Reviewer | 负责质量、品牌一致性、风险审核 |

包含 workflow：

```text
Intake
-> Reference Analysis
-> Proposal
-> Human Approval
-> Script
-> Script Review
-> Scene Plan
-> Storyboard Review
-> Asset Generation
-> Asset Review
-> Edit Decision
-> Compose / Render
-> QA Review
-> Publish Package
```

包含工具需求：

- LLM 模型账号。
- Web search。
- 图片生成 provider。
- 视频生成 provider。
- TTS provider。
- 音频/字幕工具。
- FFmpeg / Remotion / 等渲染工具。
- 云盘或知识库，用于保存脚本、分镜、素材清单、报告。

### 3.5 许可风险

OpenMontage 使用 AGPLv3。我们需要非常谨慎。

第一阶段建议：

- 可以参考它的结构、抽象和流程设计。
- 不要把上游大量 skill、manifest、schema、代码直接打包进商业发行物。
- 如果未来要 vendor 内容，需要明确许可证策略、署名、NOTICE、是否触发 AGPL 义务。
- 更稳妥的方式是做 OpenMontage-compatible playbook generator：让用户自己连接或安装 OpenMontage，而不是我们内置复制其资产。

也就是说：

```text
OpenMontage 第一版更适合做结构参考，不适合像 OpenSpec 那样直接搬完整 skill。
```

## 4. n8n 给我们的启发

n8n 的模板不是 agent playbook，但它证明了一个事实：

```text
用户很喜欢从具体业务模板开始，而不是从空白画布开始。
```

n8n 里的 AI、sales、support、marketing、document processing 模板可以作为 Multigent 的行业入口参考。

但 Multigent 不能只复制 n8n：

| n8n Workflow | Multigent Playbook |
| --- | --- |
| 强调 trigger/action 自动化 | 强调人、agent、工具、审核协作 |
| 节点多是 API 集成 | 节点可以是 agent 或 human |
| 输出常是一次动作 | 输出应是可审计 artifact |
| 失败常靠重试 | 失败要沉淀为 skill / policy |

因此我们更应该把 n8n 的模板当作“外部工具编排样本”，而不是直接当作 playbook。

## 5. 当前 Multigent 的内置方案问题

当前内置协作方案主要在代码里：

- `internal/playbook/templates.go`
- `internal/playbook/workflows.go`
- `internal/playbook/*_assets.go`
- `internal/playbook/*_assets/`

这种方式早期可接受，因为：

- 不依赖网络。
- 测试稳定。
- schema 和代码一起演进。
- 用户第一次启动就能看到方案。

但继续扩展会有明显问题：

- 每新增一个行业方案都要改 Go 代码。
- 多语言内容会让代码膨胀。
- 大量 skill markdown 放进 binary 会让发布包变重。
- playbook 更新必须重新发版。
- 第三方社区无法独立贡献方案。
- 许可、来源、版本、更新策略不好管理。

如果未来我们要支持几十个行业协作方案，继续写死在代码里是不合适的。

## 6. 推荐架构：Embedded Fallback + Remote Registry

建议采用混合方案。

### 6.1 Embedded Fallback

代码里保留少量稳定基础方案：

- Example Workspace / Hello Collaboration。
- Basic Agentic Delivery。
- OpenSpec Artifact Delivery。
- 未来可能还有一个通用 Process Discovery。

它们用于：

- 首次启动。
- 离线环境。
- 测试环境。
- registry 拉取失败时的 fallback。

### 6.2 Remote Registry

大量行业方案放到独立 GitHub 仓库，例如：

```text
github.com/multigent/playbooks
```

仓库结构建议：

```text
registry.json
playbooks/
  video-production-studio/
    1.0.0/
      playbook.yaml
      roles/
      skills/
      workflows/
      task-templates/
      docs/
      LICENSE
      NOTICE.md
  deep-research-studio/
    1.0.0/
      playbook.yaml
      ...
```

Multigent 启动后：

1. 读取 embedded fallback。
2. 后台拉取 remote registry。
3. 缓存到本地数据目录。
4. UI 展示本地和远程协作方案。
5. 用户安装时下载对应 bundle。
6. 校验 checksum、schema、license、compatibility。
7. 安装为 workspace 内的实际资产。

### 6.3 Registry Manifest 草案

```json
{
  "schemaVersion": 1,
  "generatedAt": "2026-07-20T00:00:00Z",
  "playbooks": [
    {
      "id": "video-production-studio",
      "version": "1.0.0",
      "name": {
        "en": "Video Production Studio",
        "zh-CN": "视频制作工作室"
      },
      "description": {
        "en": "A multi-agent video production workflow from brief to publish package.",
        "zh-CN": "从需求简报到发布包的多 Agent 视频制作流程。"
      },
      "category": "creative",
      "tags": ["video", "content", "marketing"],
      "license": {
        "type": "custom",
        "notice": "Includes original Multigent content. Inspired by public workflow patterns."
      },
      "source": {
        "type": "github",
        "url": "https://github.com/multigent/playbooks/tree/main/playbooks/video-production-studio/1.0.0"
      },
      "bundle": {
        "url": "https://github.com/multigent/playbooks/releases/download/video-production-studio-v1.0.0/video-production-studio.tar.gz",
        "sha256": "..."
      },
      "compatibility": {
        "minMultigentVersion": "0.1.0"
      },
      "locales": ["en", "zh-CN"]
    }
  ]
}
```

### 6.4 Bundle Manifest 草案

```yaml
id: video-production-studio
version: 1.0.0
schema_version: 1
category: creative
name:
  en: Video Production Studio
  zh-CN: 视频制作工作室
description:
  en: A multi-agent content production pipeline.
  zh-CN: 多 Agent 内容制作流程。
roles:
  - roles/producer.yaml
  - roles/script-writer.yaml
  - roles/editor.yaml
skills:
  - skills/video-intake/SKILL.md
  - skills/storyboard-review/SKILL.md
workflows:
  - workflows/video-production.yaml
task_templates:
  - task-templates/create-short-video.yaml
required_tools:
  - web_search
  - image_generation
  - tts
  - video_generation
  - object_storage
license:
  type: custom
  files:
    - LICENSE
    - NOTICE.md
```

### 6.5 安全与信任

Remote registry 不能变成任意代码执行入口。

第一版限制：

- Playbook bundle 只能包含 data：YAML、JSON、Markdown、图片说明、文档。
- 不能直接包含可执行脚本。
- Skill 中可以描述工具使用方式，但实际可调用工具必须映射到 Multigent 已知 tool adapter。
- 安装前必须校验 schema。
- bundle 必须有 checksum。
- 后续可以增加签名校验。

这点很重要，因为 playbook 会影响 agent 的行为，等同于组织级 prompt 和流程配置。

## 7. 安装、更新、修改策略

### 7.1 安装

安装 playbook 后生成 workspace 内的实例资产：

```text
Playbook Template
-> Team / Role / Skill / Workflow / Task Template instances
```

每个实例资产记录来源：

```yaml
source:
  type: playbook
  playbook_id: video-production-studio
  playbook_version: 1.0.0
  asset_id: script-writer
  installed_at: 2026-07-20T00:00:00Z
  customized: false
```

### 7.2 用户修改

用户修改安装后的资产时：

- 标记 `customized: true`。
- 后续 playbook 更新不自动覆盖。
- UI 提示“该资产已由你修改，后续不会自动接收模板更新”。

第一版不做 diff / merge。复杂合并会明显拖慢产品。

### 7.3 更新

第一版可以只支持：

- 查看有新版本。
- 安装新版本为新的资产副本。
- 不覆盖已修改资产。

后续再做：

- managed asset 自动更新。
- detached asset 保持不变。
- 手动对比差异。

### 7.4 卸载

第一版可以不支持完整卸载。

原因：

- playbook 安装后会生成多个实体，可能已经被项目、任务、agent 引用。
- 自动删除容易误伤正在使用的流程。

更合理的第一版能力：

- 展示“来源于哪个 playbook”。
- 用户可以在各资产页面手动删除。
- 后续再做“安全卸载检查”。

## 8. 对当前代码的建议迁移路径

不要一次性把所有内置方案改成远程拉取。建议分阶段：

### Phase 1：把模板从 Go 代码迁到本地数据文件

目标：

- `internal/playbook/templates.go` 不再写大量长 prompt。
- 模板变成 `playbooks/builtin/<id>/playbook.yaml`。
- Go 代码只负责 loader、validator、installer。

好处：

- 结构先解耦。
- 不引入网络复杂度。
- 测试仍然稳定。
- 方便编辑和 review。

### Phase 2：支持 Embedded Data Bundle

把 `playbooks/builtin/` 作为 embedded FS。

这保持离线可用，同时避免 Go 代码膨胀。

### Phase 3：支持 Remote Registry

新增：

- registry fetch。
- cache。
- checksum。
- schema validation。
- source/license metadata。
- UI 展示远程方案。

### Phase 4：独立 Playbook 仓库

建立：

```text
github.com/multigent/playbooks
```

把 OpenMontage-style、Deep Research、Sales Ops、Support Ops 等非核心方案放进去。

Multigent 主仓库只保留：

- schema。
- registry client。
- installer。
- 少量 fallback templates。

## 9. 第一批非研发方案建议

优先级建议：

| Priority | Playbook | 原因 |
| --- | --- | --- |
| P0 | Video Production Studio | OpenMontage 结构完整，视觉冲击强，能证明 Multigent 不只是研发工具 |
| P0 | Deep Research Studio | 通用、高频、容易验证，适合市场/战略/销售/产品 |
| P1 | Support Reply Review | 低风险、高频，企业容易理解 |
| P1 | Sales Lead Follow-up | 商业价值直接，但外发风险要 human gate |
| P1 | Meeting-to-Action Loop | 适合办公场景，跟飞书/Lark 工具连接契合 |
| P2 | Marketing Campaign Studio | 需要更多外部工具和品牌上下文 |
| P2 | Design Review Studio | 依赖 Figma / brand guideline /视觉质量判断 |

第一版最建议做：

```text
Video Production Studio + Deep Research Studio
```

原因：

- 一个偏创意生产，一个偏知识生产。
- 都不是软件开发。
- 都能自然展示 workflow、artifact、human review、tool binding。
- 都适合做 demo workspace。

## 10. 当前决策建议

### 10.1 OpenMontage

建议先把 OpenMontage 作为研究和设计输入，不要马上完整 vendor。

可做：

- 设计 `Video Production Studio` 协作方案。
- 复用它的 pipeline/stage/artifact/gate 思路。
- 不直接复制 AGPL 内容进商业发行包。

### 10.2 内置方案分发

建议采用：

```text
Embedded fallback + GitHub remote registry
```

不要纯远程，因为：

- 初次启动必须可用。
- 企业内网可能无法访问 GitHub。
- 测试需要确定性。

不要纯内置，因为：

- 内容会快速膨胀。
- 方案更新不应该绑定二进制发布。
- 社区贡献和行业模板扩展需要独立仓库。

### 10.3 下一步

建议按这个顺序推进：

1. 把当前 Go 内置 playbook 抽成本地 data bundle。
2. 定义 playbook registry schema。
3. 做 `Video Production Studio` 的 Multigent 原创版 playbook 草案。
4. 做 remote registry fetch/cache。
5. 在 UI 上展示来源、版本、许可证、是否可更新。

这条路能同时解决两个问题：

- 我们可以快速扩展更多行业协作方案。
- Multigent 主仓库不会因为 playbook 内容越来越多而臃肿。

## 11. 参考来源

本轮调研参考了以下项目和资料：

| 来源 | 用途 |
| --- | --- |
| [OpenMontage](https://github.com/calesthio/OpenMontage) | 视频制作协作方案、pipeline/stage/artifact/checkpoint/human gate 设计参考 |
| OpenMontage 本地仓库 `AGENT_GUIDE.md` | agent 如何按 pipeline manifest 和 stage director skill 工作 |
| OpenMontage 本地仓库 `PROJECT_CONTEXT.md` | instruction-driven 架构、artifact、tool registry、pipeline 列表 |
| OpenMontage 本地仓库 `pipeline_defs/` | 视频制作流程阶段、checkpoint、approval gate、required artifacts |
| OpenMontage 本地仓库 `skills/` | stage director skill、meta skill、creative skill 组织方式 |
| [n8n AI workflow templates](https://n8n.io/workflows/categories/ai/) | 企业自动化模板分类和常见场景 |
| [n8n Sales workflow templates](https://n8n.io/workflows/categories/sales/) | 销售线索、跟进、报价等销售协作场景 |
| [awesome-n8n-templates](https://github.com/enescingoz/awesome-n8n-templates) | 社区自动化模板仓库形态参考 |
| [GPT Researcher](https://github.com/assafelovic/gpt-researcher) | 深度研究 agent 场景参考 |
| [Open Deep Research](https://github.com/langchain-ai/open_deep_research) | 可配置、多 provider 的 research agent 参考 |
