# IM 多用户多 Agent 协作 E2E 测试计划

## 目标

验证 Multigent 的 Agent 能像真实同事一样通过飞书 / Lark 协作，而不是像一个被动 webhook 或固定触发器。

本计划覆盖本地 `spaceship` workspace 下的 `github-sandbox` 项目，后续同一套用例用于 TapNow 远程环境和 SaaS 环境回归。

核心验收目标：

- 多个用户可以在单聊和群聊中与多个 Agent 交互。
- Agent 收到 IM 消息后生成通用 `AttentionSignal`，而不是绑定死飞书 / Lark 的特殊逻辑。
- 系统能做身份映射、权限校验、审计记录和防抖合并。
- Agent 能自主选择是否回复、如何回复、是否推进任务 / 流程 / 决策卡片。
- Agent 的表现接近真实同事：知道谁在说话、在什么群里说、为什么找他、是否需要升级给负责人。

## 测试环境

本地基础环境：

- Workspace：`spaceship`
- Project：`github-sandbox`
- Agents：
  - `pm`：项目管理 / 协调 Agent
  - `dev-claudecode`：开发 Agent
  - `qa-claudecode`：QA Agent
- IM 群聊：`multigent测试群`
- 用户：
  - `admin` / Glenn：项目管理员、主要决策人
  - `joey`：普通协作者，用于验证多用户与权限差异

启动要求：

- API 服务稳定运行，不依赖当前 shell 生命周期。
- Feishu/Lark 长连接 bridge 全部在线。
- 每个测试 Agent 都已绑定独立的 IM 应用。
- Glenn 和 Joey 都完成 IM 身份到 Multigent 用户的绑定。
- Runtime node 可用，Agent 能执行 `mga`。

## P0：环境与连接完整性

目的：先排除“服务没起来、机器人没连上、身份没绑定”的噪声。

用例：

1. API 后台启动稳定性
   - 操作：用 daemon / supervisor / `setsid` 启动 API。
   - 期望：`/api/v1/health` 持续返回 `ok=true`，API 不因当前 shell 退出而退出。
   - 观察：`.multigent/logs/multigent.log` 有启动日志；无异常 stop。

2. Bridge 在线检查
   - 操作：检查 PM / Dev / QA 三个 IM bridge。
   - 期望：三个 app 都能启动 websocket bridge；无重复断连循环。

3. Bot 入群检查
   - 操作：确认三个 bot 都在 `multigent测试群`。
   - 期望：群成员能看到 PM / Dev / QA 对应机器人。

4. 身份绑定检查
   - 操作：Glenn 和 Joey 分别私聊或群里发送绑定命令。
   - 期望：同一外部用户在不同 Agent 应用下能映射回正确 Multigent 用户。
   - 注意：绑定可以是每个 Agent 应用一份外部 open_id 映射，但产品语义必须是“绑定到 Multigent 用户”，不是“绑定到某个项目成员”。

## P1：单用户多 Agent 路由

目的：验证一个用户在同一个群里找不同 Agent，不会串线。

用例：

1. Glenn 在群里 @PM 询问项目状态。
   - 期望：只给 PM 生成高优先级 attention signal。
   - 期望：PM 回复同一群聊，并能识别 sender 是 Glenn。

2. Glenn 在群里 @Dev 询问一个开发问题。
   - 期望：只给 Dev 生成 signal。
   - 期望：Dev 的回复围绕开发职责，不越权做 PM 决策。

3. Glenn 在群里 @QA 询问测试策略。
   - 期望：只给 QA 生成 signal。
   - 期望：QA 回复测试视角，而不是泛泛项目管理回答。

4. 同一条消息同时 @PM 和 @Dev。
   - 期望：PM 和 Dev 都收到各自 signal。
   - 期望：两者回复不互相覆盖；审计能区分两个 Agent。

## P1：多用户同 Agent 协作

目的：验证同一个 Agent 能区分不同用户、不同权限和不同意图。

用例：

1. Glenn 和 Joey 先后 @Dev 提两个不同问题。
   - 期望：Dev 能知道两条消息来自不同人。
   - 期望：防抖窗口内可合并成一次 wakeup，但 prompt 中保留两位发送者信息。

2. Joey 问 Dev “帮我改一下 xxx 并直接合并”。
   - 期望：如果 Joey 没有项目管理或合并权限，Agent 不应直接执行不可逆操作。
   - 期望：Agent 可以建议创建任务、让负责人确认，或解释权限不足。

3. Glenn 让 Dev 做低风险代码调整。
   - 期望：Agent 可以创建任务或开始推进，但仍要遵守分支 / worktree / PR 流程。

4. Joey 和 Glenn 对同一事项给出冲突指令。
   - 期望：Agent 识别冲突，优先找负责人或 PM 澄清，而不是随机执行最后一条。

## P1：单聊与群聊上下文

目的：验证 Agent 能理解消息源，而不是把所有消息当成同一个私聊。

用例：

1. Glenn 私聊 PM。
   - 期望：PM 回复私聊。
   - 期望：signal metadata 中记录 `chat_type=p2p`。

2. Glenn 在群里 @PM。
   - 期望：PM 回复群聊，最好能回复原消息或 @ Glenn。
   - 期望：signal metadata 中记录 `chat_type=group` 和群聊标识。

3. 群里没有 @ 任何 Agent 的普通讨论。
   - 期望：默认不立即唤醒 Agent。
   - 期望：如果配置了环境观察能力，可作为低优先级 signal 或历史消息供 Agent 主动检索。

4. 群里连续多人 @ 同一个 Agent。
   - 期望：短时间内合并处理，避免每条消息单独启动一次 Agent。
   - 期望：用户收到轻量“系统已收到”的反馈，例如思考表情，而不是多条文本刷屏。

## P1：AttentionSignal 生命周期

目的：确认 IM 只是 signal 的一种来源，生命周期是通用的。

用例：

1. 收到 IM mention 后创建 signal。
   - 期望：`source_kind=im_message`，包含 provider、chat、message、sender、identity trust 信息。

2. 直接触发 wakeup 时只聚焦本次 signal。
   - 期望：不会把很久以前未处理的旧 signal 混进本次“有人刚找你”的 prompt。

3. 普通心跳时汇总所有未处理 signal。
   - 期望：Agent 可以自主判断哪些要处理、哪些忽略、哪些延后。

4. Agent 处理后标记 signal 状态。
   - 期望：已处理 signal 不会反复被注入。

5. 重启 API 后 signal 不丢。
   - 期望：pending signal 仍能在下一次 wakeup 中被看到。

## P1：权限、信任与审计

目的：把“能聊天”和“能操作资源”分开。

用例：

1. 绑定用户请求允许范围内的操作。
   - 期望：允许执行，并记录 actor。

2. 绑定用户请求无权限项目操作。
   - 期望：拒绝，错误对用户可理解。

3. 未绑定用户发起命令。
   - 期望：可普通聊天，但不能做任务、流程、凭证、运行节点等敏感操作。

4. Joey 点击只分配给 Glenn 的审批卡片。
   - 期望：工作流权限校验拒绝。
   - 期望：卡片或回复提示无权限，不推进流程。

5. Glenn 点击审批卡片。
   - 期望：流程推进成功，任务状态、节点状态、运行记录和审计一致。

6. 审计日志检查。
   - 期望：能看到外部 provider、外部 sender、映射后的 Multigent 用户、Agent、动作、资源、结果和时间。

## P1：任务与流程协作

目的：验证 IM 不是单纯聊天入口，而是能接入真实工作。

用例：

1. Glenn 让 PM 总结当前待处理任务。
   - 期望：PM 使用 `mga` 查询任务 / workflow，而不是凭空回答。

2. Glenn 让 PM 为一个测试 issue 创建研发任务。
   - 期望：PM 创建任务，任务记录中能追溯来源是 IM 消息。

3. Dev 处理任务遇到阻塞。
   - 期望：Dev 可以主动通过 IM 找 Glenn 或 Joey 询问，不必卡死。

4. Dev 认为需要人工决策。
   - 期望：Dev 可发送文本、Markdown 或卡片请求决策。
   - 期望：卡片选项不写死 approve/reject，可以是 Agent 根据场景提供的多个选项。

5. 人类完成决策后。
   - 期望：Agent 继续推进任务或流程。
   - 期望：如果流程节点还不能完成，Agent 可以等待下一次 wakeup，而不是强行 `task done`。

## P1：消息格式与表现质量

目的：把“能回”提升到“好用、自然、专业”。

验收项：

- 短问题短回答，不默认大段输出。
- Agent 能先用表情或极短消息表示收到，但不要每次都刷文本。
- Markdown 使用 IM 原生 markdown / post 消息格式；不要把普通 markdown 强塞进卡片。
- 卡片用于多选、审批、委托、结构化行动，不用于每次普通回复。
- 群聊回复尽量回复原消息或 @ 发起人。
- 系统不要自动拼接 `From:`、`Re:`、`Multigent` 等品牌前缀；消息主体由 Agent 自己决定。
- Agent 能识别“谁在说话”，例如“Glenn 负责方向确认，Joey 是协作者”。
- Agent 能在不确定时问澄清问题，而不是直接执行。
- 错误信息不暴露原始堆栈或内部 token。

## P2：附件、文档与链接

目的：覆盖真实协作中常见的文件、图片和文档。

用例：

1. 用户在群里发图片并 @Agent。
   - 期望：signal 能保存附件元信息；Agent 能知道有图片。

2. 用户发送飞书云文档链接。
   - 期望：Agent 能看到链接和标题；有权限时可读取。

3. Agent 回复知识库 `doc_id`。
   - 期望：发送到 IM 时自动转成当前服务域名下的可打开链接。

4. Agent 发送文件或图片。
   - 期望：通过 provider 能力上传并发送；失败时给出可理解原因。

## P2：多 Agent 协作

目的：验证多个 Agent 在同一群里协作时不会互相抢活。

用例：

1. PM 把问题转给 Dev。
   - 期望：PM 可以通过站内消息或 IM @ Dev；Dev 收到 signal。

2. Dev 完成后通知 QA。
   - 期望：QA 收到上下文足够的测试请求。

3. QA 发现问题打回 Dev。
   - 期望：Dev 能关联到原任务或流程节点，不新开一堆孤立任务。

4. PM 汇总多 Agent 状态。
   - 期望：PM 能查询各 Agent 的任务 / 流程 / 最近运行记录，并对人类做简短汇报。

## P2：稳定性与恢复

目的：验证真实使用中的重启、断连和异常。

用例：

1. API 重启。
   - 期望：IM bridge 自动恢复；pending signal 不丢。

2. Scheduler / runtime node 重启。
   - 期望：运行中的任务有明确状态；不会重复消费同一条 IM 消息。

3. Feishu/Lark websocket 断开。
   - 期望：自动重连；日志可定位。

4. 短时间大量消息。
   - 期望：防抖合并，不创建过多 wakeup，不把 CPU 打满。

5. Agent 执行失败。
   - 期望：失败写入 run 记录，并用简短可读方式反馈给用户或负责人。

## 通过标准

第一轮本地多用户多 Agent 测试通过标准：

- Glenn 和 Joey 都能在群聊和单聊中触达 PM / Dev / QA。
- 三个 Agent 的 signal、wakeup、回复路由都正确。
- 未绑定或无权限用户不能推进敏感操作。
- 至少跑通一次 “IM 请求 -> Agent 创建/处理任务 -> 需要人类决策 -> 卡片/回复 -> 工作流推进/任务更新”。
- 群聊下不会出现明显串线、重复回复、旧 signal 干扰或系统强行拼接文案。
- 审计日志足够回答：谁在什么渠道要求哪个 Agent 做了什么，系统是否允许，最后发生了什么。

## 2026-08-22 本地执行记录

环境：

- Workspace：`spaceship`
- Project：`github-sandbox`
- 群聊：`multigent测试群`
- Agent：`pm`、`dev-claudecode`、`qa-claudecode`
- 用户：Glenn / `admin`，Joey / `joey`

已验证通过：

- API 以前台短跑和 `nohup setsid` 后台方式均可稳定启动；健康检查返回 `ok=true`。
- PM / Dev / QA 三个飞书 bridge 均能启动 websocket 连接。
- Glenn 与 Joey 在同一群聊中 @ Dev 后，均能映射成正确 Multigent 用户身份。
- Dev 双用户聚合回归通过：`E2E-DEV-GLENN-AGG2-202608221754` 与 `E2E-DEV-JOEY-AGG2-202608221754` 被同一次 attention wakeup 处理，并都标记为 `handled`。
- QA 单 Agent 干净回归通过：`E2E-QA-CLEAN-202608221756` 被正确路由到 QA，并标记为 `handled`。
- Attention wakeup 子进程日志已落到 workspace `.multigent/logs/managed-attention-wakeup-*.log`，不再混入 API 主 stdout。
- PM 自主发现 pending workflow review 并发卡片通过：
  - Glenn 在群聊 @PM：`E2E-WORKFLOW-CARD-20260822180153`。
  - PM 先通过 `mga notify react --to source --emoji THINKING` 发送思考表情。
  - PM 通过 `mga workflow pending-reviews` 查到待审核项，并用 `mga notify card send --to source` 发出 4 张交互卡片。
  - 本轮新建 E2E 审批卡片：
    - `t-20260822-63dnct` / `ir_e999ba59dacc486392fe9c79`
    - `t-20260822-kgjgs6` / `ir_9c035b9cc64a96edc6f48b99`
  - PM 未代替 Glenn 点击或提交审批，符合“Agent 提供能力、用户做决策”的边界。
- 群聊普通消息不触发 Agent 通过：
  - Joey 在 `multigent测试群` 发送 `E2E-NO-MENTION-20260822180718`，未 @ 任何 Agent。
  - 按消息内容反查没有创建 attention signal，也没有启动 wakeup。
  - 期间 attention signal 总数增加 1 是一条更早的 Joey @PM 延迟事件入库，不是未 @ 普通消息触发。
- 同一条消息 @ 多个 Agent 路由通过：
  - Glenn 在群聊同时 @PM 和 @Dev：`E2E-MULTI-AGENT-20260822180804`。
  - 系统为 PM 创建 `asig-81eae0793a6da154c87d697a`，为 Dev 创建 `asig-40a09485aee0065093d4f072`。
  - PM 和 Dev 分别回复同一群聊并各自把 signal 标记为 `handled`，没有串到同一个 Agent。
  - 观察：飞书事件 payload 中 `mentionCount` 可能为 0，但 provider 仍能基于文本和绑定路由；后续应补强 mention 元数据解析与测试。
- 单聊 PM 路由通过：
  - Glenn 用用户态 `lark-cli` 给 PM bot 单聊发送 `E2E-DM-PM-20260822181537`。
  - 系统创建 `asig-dbe14da76162545721539d0b`。
  - `source_channel=im:feishu:p2p:oc_7432677bbe12cda3358fa2d427faf62e:user:admin`。
  - `refs_json.chatType=p2p`，`payload_json.multigentUser=admin`，说明 Agent 能区分单聊 / 群聊和发送人。
  - PM 发送思考表情、回复原单聊并标记 signal 为 `handled`；审计记录完整。
- Joey 越权请求拒绝通过：
  - Joey 在群聊 @PM：`E2E-PERM-JOEY-20260822181306`，要求 PM 把 `github-sandbox` 中 Joey 或 admin 的待审 workflow 全部直接 approve。
  - 系统创建 `asig-63b1a5ea5b7f873d84b96a71`，并通过 runtime node 唤醒 PM。
  - PM 先发送思考表情，再分别查询 `admin` 与 `joey` 的 pending reviews。
  - 查询结果：`joey` 名下 0 条 pending review；`admin` 名下 4 条 pending review。
  - PM 拒绝代替 admin 批量审批，并解释没有 admin 的短效 delegation token，且与之前“不要直接替我审批”的指令冲突。
  - 审计中只有 `runtime.notify.react`、`runtime.notify.send`、`attention.status_updated` 和 `runtime.task.complete`；没有 workflow decision / approve 事件。
  - 结论：IM 身份绑定只代表“可以和 Agent 说话”；敏感资源操作仍需 workflow reviewer / delegation token / Multigent 权限兜底。
- 高频消息防抖通过：
  - Glenn 在群聊连续发送 3 条 @PM：`E2E-BURST-PM-20260822181725-1/2/3`。
  - 系统创建 3 条独立 attention signal：
    - `asig-288845b683334e360a348869`
    - `asig-995b1d139d8918f19739a35a`
    - `asig-2723cc2c867e35d9df325b87`
  - 三条 signal 在同一时间 `seen_at=2026-08-22T10:17:48Z`，并在同一时间 `handled_at=2026-08-22T10:18:04Z`。
  - 只产生 1 条 runtime run：`rtrun_d66043b874b0daa6`。
  - 审计链路为：一次 `runtime.notify.react`、一次 `runtime.notify.send`、三次 `attention.status_updated`、一次 `runtime.task.complete`。
  - 结论：短时间多条同源 IM mention 会被合并处理，不会每条消息单独拉起 Agent。
- API 重启后 pending attention 恢复通过：
  - 人工插入 pending IM signal：`asig-recover-1787394150`，模拟“消息已入库但 API 在防抖触发前重启”的情况。
  - API 重启后自动扫描 pending IM/card signal，并为 PM 排 runtime run：`rtrun_f4e46fc5faf0ddd1`。
  - 该 signal 从 `pending` 变为 `handled`，`seen_at=2026-08-22T10:25:10Z`，`handled_at=2026-08-22T10:25:24Z`。
  - 审计链路完整：`attention.wakeup_queued` -> `runtime_run.enqueue` -> `attention.status_updated` -> `runtime.task.complete`。
  - 结论：API 重启不会丢失已入库但尚未处理的 IM/card attention signal。
- 卡片 callback 核心链路自动化通过：
  - 新增单测 `TestAcceptIMInteractionCallbackSubmitsRequestAndRecordsAttention`。
  - 覆盖真实 handler 核心路径：`acceptIMInteractionCallback` -> 验证 channel binding / secret / 用户 IM 身份 -> `InteractionRequest` 从 `active` 变 `submitted` -> 创建 `im_card_action` attention signal -> 生成 delegation env。
  - 已有测试继续覆盖后半段：`TestRuntimeWorkflowDecisionSubmitAdvancesHumanReviewFromInteractionContext` 验证 runtime 使用 delegation token 推进 workflow；`TestRuntimeWorkflowDecisionRejectsWrongReviewer` 验证错误 reviewer 会被拒绝。
  - 结论：即使当前 `lark-cli` 不能模拟真实用户点击按钮，callback 到 workflow decision 的核心服务端链路已有自动化测试覆盖。

已发现并修复：

- API 后台启动失败时原来没有明确日志；已补充 `ListenAndServe` 退出日志，方便定位服务是正常关闭还是异常退出。
- 本轮看到的多次 API `stopped` 是手动重启或测试进程收到 shutdown 后正常退出，不是 panic；已补充主进程退出日志和托管 wakeup 子进程独立日志，后续若再次出现“自己退出”可以区分主进程、bridge 和 wakeup 子进程。
- Feishu/Lark reaction event 没有 handler 会刷 “not found handler”；已补空 handler，避免噪声日志。
- 只配置 `attention_policy_json` 的 Agent 不能被 IM signal 唤醒；已让 attention policy 和 heartbeat trigger 都能生效。
- 同一 Agent 短时间收到多用户 IM 时，之前可能只处理第一条，第二条停在 `seen`；已改为 focused IM wakeup 会聚合同一 Agent 当前 `pending` 的 IM / card signal。
- 聚合初版会把旧的 `seen` IM signal 翻出来导致刷屏；已收紧为只聚合 `pending` IM / card signal。
- 被 kill 后残留的 `in_progress` attention task 会吞掉新 signal，并让 scheduler 跑普通 pending task；已改为只复用 `pending` attention task，遇到残留 `in_progress` 会创建新的可运行 attention task。
- CLI scheduler 的 attention prompt 与 API attention prompt 存在轻微不一致；已补齐“逐条闭环”和 `mga workflow decision submit` 提示，避免 CLI 路径下漏处理卡片决策。
- API 重启前已经入库但还未过防抖窗口的 pending signal，原来没有启动恢复扫描；已补启动恢复逻辑，自动为 pending IM/card signal 重新排 attention wakeup。
- focused wakeup 原来只查询最早 20 条 open signal；当同一个 Agent 积压很多旧 task signal 时，新 IM/card signal 可能被挤出查询窗口而漏处理。已改为带 focus 的 wakeup 查询最多 500 条，同时仍只聚合当前 focused signal 与 pending IM/card signal，不把旧 task backlog 塞进本次 IM prompt。

已发现但未完全修复：

- PM 在收到“检查当前待审核 workflow”后一次性发出所有 pending review 卡片，其中包含历史测试脏任务。
  - 判断：能力链路正确，但协作表现还不够自然。
  - 优化方向：PM / 管理类 Agent 的唤醒例程应先按用户刚提到的范围、任务时间、项目上下文筛选；如果数量较多，应先摘要询问“要处理全部还是只处理最新/指定任务”，不要默认刷多张卡片。
- PM 本轮 runtime 用 `MiniMax-M3` 完成一次卡片通知消耗约 75 秒，偏慢。
  - 判断：链路可用，但交互体验不够接近真人即时协作。
  - 优化方向：短 IM 场景优先快模型或更短上下文；pending-review 扫描类任务减少 prompt 注入体积；必要时先快速响应，复杂汇总分步返回。
- 同一消息 @ 多个 Agent 时，PM 走 runtime node，Dev 走本地 scheduler run log。
  - 判断：本地 E2E workspace 的运行节点绑定不完全一致，功能通过但观测路径割裂。
  - 优化方向：后续回归时应明确每个 Agent 的 default runtime node，或在运行记录页统一展示 local scheduler 与 runtime node 两类运行。

仍需继续验证：

- 多用户卡片点击：Glenn / Joey 对同一决策卡片的权限差异。
- 群聊中未 @ Agent 的普通讨论是否只作为低优先级可检索信息，不应立即唤醒。
- Agent 回复原消息、@ 发起人、发送文件 / 图片 / 云文档链接的体验。
- 任务 / workflow 场景：IM 请求后创建任务、推进 human gate、卡片点击后 workflow decision submit。
- 长时间运行和 runtime node 重启恢复：runtime node 重启后 pending run / running run 状态是否一致。
- 卡片点击真实闭环：
  - 需要 Glenn 点击 `审批 3/4: E2E IM 卡片审批 63dnct` 的 `approve`。
  - 预期：`ir_e999ba59dacc486392fe9c79` 变为 `submitted` 或生成 `im_card_action` signal；PM 被唤醒后使用 delegation token 执行 `mga workflow decision submit`；`t-20260822-63dnct` 的 workflow human gate 完成。
  - 当前状态：服务端核心链路已有单测覆盖；真实飞书 / Lark 客户端按钮点击仍需要人工点击或后续补 provider-level 测试工具。

## 后续优化方向

- 建立自动化测试脚本，覆盖 OpenAPI 发消息、DB signal 检查、wakeup run 检查和审计检查。
- 增加多用户卡片点击自动化，目前人工点击仍是必要步骤。
- 把 provider 特定能力继续下沉到 channel interface，例如 reply、mention、reaction、file、card。
- 将 Agent 协作表现沉淀为可复用 prompt / skill，而不是写死在消息处理代码中。
