# 配置与日志

## 配置来源

长期运行命令支持 TOML 配置文件：

```bash
multigent --config ./config.toml start
multigent --config ./config.toml api serve
multigent --config ./config.toml daemon install
```

优先级：

```text
CLI 参数 > 环境变量 > config.toml > 默认值
```

也可以通过环境变量指定全局配置文件：

```bash
MULTIGENT_CONFIG=/etc/multigent/config.toml
```

示例配置见 [config.example.toml](../config.example.toml)。

当前配置覆盖：

- workspace 数据目录
- server 监听地址
- API key
- SMTP 邀请邮件
- 服务日志
- 自托管 E2B API URL
- 远程协作方案 registry

## 日志策略

Multigent 有两类日志：

- 服务日志：API/Web 进程生命周期和平台事件。
- Agent 运行日志：每次 agent 执行的 stdout/stderr 和对话转录。

服务日志默认：

- 路径：`~/.multigent/logs/multigent.log`
- 格式：JSON
- 级别：`info`
- 轮转：保留一个 `<log>.1`
- 默认大小：`10 MB`

生产环境推荐：

```toml
[logging]
file = "/var/log/multigent/multigent.log"
level = "info"
format = "json"
max_size_mb = 100
stderr = false
```

Agent 运行日志仍然归属到对应 agent workspace：

```text
projects/<project>/agents/<agent>/.multigent/runs/
```

服务日志应保持结构化，便于汇总到中心日志系统；运行日志是某次执行的产物，主要用于回放和排障。

## 常用环境变量

日志：

- `MULTIGENT_LOG_FILE`
- `MULTIGENT_LOG_LEVEL`
- `MULTIGENT_LOG_FORMAT`
- `MULTIGENT_LOG_MAX_SIZE_MB`
- `MULTIGENT_LOG_STDERR`

服务：

- `MULTIGENT_SERVER_ADDR`
- `MULTIGENT_API_ADDR`
- `MULTIGENT_WEB_API_KEY`

SMTP：

- `MULTIGENT_SMTP_HOST`
- `MULTIGENT_SMTP_PORT`
- `MULTIGENT_SMTP_USERNAME`
- `MULTIGENT_SMTP_PASSWORD`
- `MULTIGENT_SMTP_FROM`
- `MULTIGENT_SMTP_FROM_NAME`
- `MULTIGENT_SMTP_TLS`

Sandbox：

- `MULTIGENT_E2B_API_URL`

协作方案：

- `MULTIGENT_PLAYBOOK_REGISTRY_URLS`
