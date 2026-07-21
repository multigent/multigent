# GitHub 协作开发与发版流程

本文基于旧 Spaceship workspace 中 `projects/cc-connect/playbooks/*.md` 梳理，目标是沉淀成 Multigent 可复用的流程模板和协作方案。这里保留的是流程思想，不保留旧 `agencycli` 命令和本地路径假设。

## 总体设计

这套流程围绕 GitHub 开源项目维护，核心对象有三个：

- Issue：用户反馈、bug、需求、咨询和 release blocker 的入口。
- PR：开发 agent 的主要交付物，也是 QA review 的对象。
- Release Scope：准备发版时的候选变更集合、风险证据和 human confirm 项。

它不是一个单线流程，而是三套互相衔接的流程：

1. Issue Triage → Dev Task → PR
2. PR Review → QA Gate → Ready to Merge
3. Release Scope → Readiness Report → Human Confirm

关键原则：

- PM 只负责 issue 分流和用户可见回复，不做 PR review。
- Dev 只在明确任务下工作，不自行巡检代码库。
- QA 只围绕 PR review、release gate、测试风险工作。
- Release Coordinator 只准备发版判断材料，不自动发布。
- merge、tag、release、公开外发、商业承诺都必须 human confirm。

## 流程一：Issue 到 PR

适用场景：GitHub issue 进入项目，需要判断是直接回复、补充信息、派开发，还是升级给人。

### 节点

| 节点 | Actor | 输入 | 输出 |
| --- | --- | --- | --- |
| GitHub Issue Sync | PM Agent | GitHub open issues、issue registry | 最新 issue inbox、未回复列表 |
| Issue Triage | PM Agent | issue 标题、正文、评论、标签、历史状态 | triage 分类、优先级、下一步 |
| User Reply | PM Agent | triage 结论、用户语言、可公开信息 | GitHub issue comment |
| Dev Task Dispatch | PM Agent | bug/feature issue、证据、验收标准 | 指派给 dev agent 的结构化任务 |
| Implementation | Dev Agent | dev task、关联 issue、项目规范 | 分支、代码改动、测试结果 |
| PR Creation | Dev Agent | 代码提交、验证结果 | PR，描述包含 `Fixes #issue` 或 `Closes #issue` |
| PM Registry Update | PM Agent | dev 反馈、PR 链接 | issue 状态变为 `has-pr`，触发 QA |

### 主要分支

| 条件 | 流向 |
| --- | --- |
| PM 能确定答案 | Issue Triage → User Reply → 结束或等待用户补充 |
| 信息不足 | Issue Triage → User Reply，索取复现信息 → 等用户 |
| 明确 bug/开发任务 | Issue Triage → User Reply → Dev Task Dispatch |
| 已有 PR | Issue Triage → PM Registry Update → PR Review 流程 |
| release-blocker/P0 | Issue Triage → Human Confirm + 视情况派 dev/QA |
| business/owner-decision | Issue Triage → Human Confirm 或 BizDev |

### Dev Task 输入规范

PM 派给开发 agent 的任务必须结构化，至少包含：

```text
问题:
证据:
期望行为:
非目标:
验收标准:
风险:
关联 issue:
完成后:
```

这比自然语言“修一下这个 issue”更适合 agent 执行，因为它减少猜测，便于后续 QA 对照验收。

### Dev 输出规范

开发 agent 完成时必须输出：

- PR 号和链接。
- 关联 issue 号。
- 分支名。
- 改动文件概要。
- 验证命令和结果。
- 未覆盖风险。

Multigent 中建议把这些作为 workflow step output 字段，而不是只写在聊天总结里。

## 流程二：PR Review 与 QA Gate

适用场景：开发 agent 已创建 PR，需要判断是否可以进入 ready-to-merge，或打回开发。

### 节点

| 节点 | Actor | 输入 | 输出 |
| --- | --- | --- | --- |
| PR Inbox Sync | QA Agent | GitHub PR、CI、review registry | 待审 PR 列表 |
| PR Risk Review | QA Agent | PR diff、CI、关联 issue、风险模块 | review 结论 |
| GitHub Review | QA Agent | review 结论 | approve/comment/request-changes |
| Registry Closeout | QA Agent | GitHub review、最新 head commit | PR registry 状态 |
| Rework | Dev Agent | request-changes 评论、PR diff | 新 commit、新验证结果 |
| Ready To Merge | QA Agent | approve/comment、可接受风险 | `ready-to-merge`，waiting-on human |

### 主要分支

| 条件 | 流向 |
| --- | --- |
| 当前 head 已 review | PR Inbox Sync → Registry Closeout |
| 首次 review 无 blocker | PR Risk Review → GitHub Review approve/comment → Ready To Merge |
| 有必须修复问题 | PR Risk Review → GitHub Review request-changes → Rework → PR Risk Review |
| 作者 push 新 commit | Registry 标记 stale → PR Risk Review 重新评审 |
| 安全/凭据/数据风险 | PR Risk Review → Human Confirm + request-changes |

### QA Review 输出规范

QA 的正式结论必须写在 GitHub PR 上，而不是只写内部 note。输出字段建议：

```text
pr_number:
review_type: approve | comment | request_changes
review_url:
blockers:
suggestions:
tested:
risk_level:
next_owner:
```

### 为什么 QA 不直接 merge

旧 playbook 明确限制 QA 不 merge。原因是 merge 是不可逆且会影响发布节奏，应该由 human 或被明确授权的 release/maintainer 节点处理。Multigent 中也应该保持这个边界：QA 只能把 PR 推到 `ready-to-merge`，不能自己完成 merge。

## 流程三：Release Readiness

适用场景：准备发版，需要把 scope、PR、QA 证据、风险和 changelog 组织成 owner 可判断的材料。

### 节点

| 节点 | Actor | 输入 | 输出 |
| --- | --- | --- | --- |
| Release Scope Intake | Release Agent | release task、目标版本、候选 PR/commit | scope 草案 |
| Evidence Collection | Release Agent | PR 状态、QA gate、blocker、用户影响 | readiness evidence |
| Readiness Decision | Release Agent | scope + evidence | ready / not-ready / needs-human |
| Changelog Draft | Release Agent | merged/candidate changes | changelog/release notes 草稿 |
| Human Confirm | Human | readiness report、风险、changelog | approve / hold / change scope |
| Release Execution | Human 或授权 Maintainer | human confirm | tag/build/release |

### 发版判断标准

`ready`：

- scope 清晰。
- 无 blocker。
- QA gate 通过，或剩余风险可接受。
- changelog 和回滚条件清晰。

`not-ready`：

- 安装、启动、登录、消息收发、配置迁移、安全、数据风险中存在 blocker。
- PR/commit 状态不清。
- 没有足够验证证据。

`needs-human`：

- 是否带风险发版需要 owner 判断。
- 涉及商业承诺、赞助商权益、公开说明或重大路线取舍。

### Release 输出规范

Release agent 不发布，只输出 release readiness report：

```text
release_scope:
candidate_prs:
qa_evidence:
blockers:
known_risks:
changelog_draft:
rollback_or_hold_conditions:
human_confirmation_items:
recommendation: ready | not_ready | needs_human
```

## 推荐的 Multigent 流程模板

### 模板 A：GitHub Issue Development Loop

这是最常用的研发协作流程。

```text
Start
  → PM: Issue Triage
  → PM: GitHub User Reply
  → PM: Dispatch Dev Task
  → Dev: Implement And Open PR
  → QA: Review PR
  → Human: Merge Decision
  → End
```

回退边：

- QA request changes → Dev implement
- Human hold → PM clarify scope
- Dev blocked → PM clarify issue

### 模板 B：PR QA Gate

适合从已有 PR 直接进入 QA。

```text
Start
  → QA: Sync PR Inbox
  → QA: Review Diff And CI
  → QA: Publish GitHub Review
  → QA: Mark Ready To Merge
  → Human: Merge Decision
  → End
```

回退边：

- request changes → Dev rework
- stale head → QA re-review

### 模板 C：Release Readiness

适合 PM 或 human 发起一个发版检查任务。

```text
Start
  → Release: Collect Scope
  → Release: Collect QA Evidence
  → Release: Draft Readiness Report
  → Human: Release Decision
  → End
```

回退边：

- not-ready → PM/dev/QA 补齐 blocker
- needs-human → human 决策是否缩 scope 或延期

## 可以沉淀成的角色能力

### PM Agent

核心能力：

- GitHub issue triage。
- 用户可见回复。
- 将 issue 转为结构化 dev/QA task。
- 维护 issue registry 状态。

禁止：

- 不 review PR。
- 不 merge。
- 不 release。

### Dev Agent

核心能力：

- 基于结构化 task 创建分支。
- 最小范围实现。
- 运行测试。
- 创建带 `Fixes #issue` 的 PR。

禁止：

- 信息不足时猜需求。
- 改核心架构、安全边界、公开 API 而不升级。
- merge/release。

### QA Agent

核心能力：

- PR diff/CI/风险模块 review。
- 在 GitHub 发布正式 review。
- 将 PR 标为 ready-to-merge 或 request-changes。

禁止：

- 只写内部 note 不写 GitHub review。
- 重复 review 当前 head。
- merge PR。

### Release Agent

核心能力：

- 收集 release scope。
- 检查 QA evidence 和 blocker。
- 输出 readiness report、changelog draft、human confirm items。

禁止：

- 自动 tag/build/upload/release。
- 对外发布 release notes。

## 指标

建议在 Multigent 中记录：

- Issue 首次响应耗时。
- issue → dev task 耗时。
- dev task → PR 耗时。
- PR 创建 → QA review 耗时。
- request-changes 次数。
- PR stale re-review 次数。
- ready-to-merge → human merge/hold 耗时。
- release scope → readiness report 耗时。
- 每个阶段 token 消耗。
- human intervention 次数和原因。

这些指标能回答一个关键问题：流程是在减少 human 阻塞，还是把复杂度换了个地方。

## 第一版落地建议

先不要把所有旧 playbook 一次性做成复杂自动化。建议先做三件事：

1. 做一个 `GitHub Issue Development Loop` 流程模板，支持 PM → Dev → QA → Human 的主链路和 QA 打回。
2. 把 Dev Task、QA Review、Release Report 的输入输出字段做成固定 spec。
3. 将 GitHub、gh CLI、项目 registry 的具体接入放到外部工具/skill 层，流程层只描述状态和产物。

这样既能保留旧 `cc-connect` 里跑出来的经验，又不会把它强绑定到某个仓库、某个本地目录或某套旧 CLI。
