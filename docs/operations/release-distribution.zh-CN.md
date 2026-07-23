# 发布与分发

Multigent 第一阶段采用 CLI-first 的分发方式：

- GitHub Releases 是版本化原生二进制的来源。
- 安装脚本和 Homebrew 是主要的人类友好安装渠道。
- npm 包是一个轻量 wrapper，用来下载匹配平台的原生二进制。
- Docker 镜像用于自托管 Demo 和 Agent Runtime Sandbox。

## 发布产物

每个 `vX.Y.Z` tag 会发布不同平台的压缩包：

```text
multigent-vX.Y.Z-linux-amd64.tar.gz
multigent-vX.Y.Z-linux-arm64.tar.gz
multigent-vX.Y.Z-darwin-amd64.tar.gz
multigent-vX.Y.Z-darwin-arm64.tar.gz
multigent-vX.Y.Z-windows-amd64.zip
multigent-vX.Y.Z-windows-arm64.zip
checksums.txt
```

每个包包含：

- `multigent`：管理员/部署者 CLI 和自托管 Web Server。
- `mga`：注入 Agent Sandbox 的受控 Runtime CLI。

`mga` 必须和 `multigent` 同版本发布，否则 Docker Sandbox 内的 Agent 不能稳定完成任务、读取文档或上报流程节点。

## 安装渠道

推荐安装：

```bash
curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
```

Windows：

```powershell
irm https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.ps1 | iex
```

Homebrew：

```bash
brew install multigent/tap/multigent
```

npm：

```bash
npm install -g @multigent/multigent
```

## Docker 镜像

发布流程会推送：

```text
ghcr.io/multigent/multigent:latest
ghcr.io/multigent/multigent/runtime-base:latest
```

首次运行 Agent 最关键的是：

```text
ghcr.io/multigent/multigent/runtime-base:latest
```

这个镜像必须保持公开，否则新用户第一次运行 Docker Sandbox 就会拉取失败。发布前需要确认 GHCR package 权限为 Public。

## 发布步骤

1. 更新 `npm/package.json` 到目标版本。
2. 更新 release notes。
3. 提交版本变更。
4. 打 tag 并推送：

   ```bash
   git tag vX.Y.Z
   git push origin main --tags
   ```

5. 等待 `.github/workflows/release.yml` 完成。
6. 确认 GitHub Release 产物、GHCR 镜像、npm 包状态。
7. 验证公开 quickstart：

   ```bash
   curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
   multigent version
   mga version
   ```
