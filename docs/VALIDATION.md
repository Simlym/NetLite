# 当前验证范围与待验收项

本文按**当前 main 分支的实现和测试入口**整理。它说明仓库配置了哪些测试、哪些场景仍需真实环境验证。**不代表本次文档修改重新执行了测试，也不代表每一次 CI 都已通过。**具体运行结果请查看 [GitHub Actions](https://github.com/Simlym/NetLite/actions)。

目前实现是：Go 内置 DDNS、NetLite 独立管理 ACME / 内部 CA 证书、标准 Caddy 反向代理；Linux 默认本地管理员会话，fnOS FPK 使用统一网关，Docker 提供 host / bridge 两种模式。**已不再依赖 ddns-go 子进程或 Caddy DNS 插件。**

## 自动化验证入口

| 检查入口 | 覆盖范围 | 不覆盖的部分 |
| --- | --- | --- |
| `go test ./...`、`go test -race ./internal/...`、`go vet ./...` | Go 业务逻辑、并发及静态检查 | 公网 DNS 与设备兼容性 |
| `cd web && npm ci && npm test && npm run build` | 前端测试与生产构建 | 各浏览器、真实 fnOS 环境 |
| `scripts/smoke-linux.py`、`scripts/smoke.ps1` | 标准 Caddy、HTTP/HTTPS、应用代理和重启流程 | 公网入站和真实 Let's Encrypt |
| `scripts/smoke-panel.py`、`go test -tags webembed ./cmd/netlite` | 内嵌管理面板、账号认证与网关相关单测 | fnOS 网关实际运行身份与 Socket 权限 |
| `scripts/smoke-container.py`、`scripts/smoke-host.py` | Docker 持久化、权限、进程退出、host 网络及低端口测试 | 实际 VPS 与 ARM64 原生运行 |
| `.github/workflows/build.yml` | Linux/FPK amd64 与 arm64 产物、校验和、amd64 Linux 包烟测 | fnOS 实机安装与 ARM64 原生运行 |
| `.github/workflows/container.yml` | amd64 容器烟测；满足条件时发布 amd64/arm64 镜像 | 镜像一定已发布、生产环境一定能启动 |

`ci.yml` 在推送、PR、版本标签与手动触发时协调这些检查。构建产物和 GHCR 镜像的实际可用性取决于**对应运行成功与发布权限**，不要仅依据工作流配置断言已经发布或验收。

## 仍需手动完成的验收

| 环境 | 待验证内容 |
| --- | --- |
| **fnOS 实机** | FPK 安装、登录态、管理员权限、Unix Socket 连接权限、重启、升级和卸载，以及系统 Web 服务不受影响 |
| **ARM64 设备** | 实际启动、服务生命周期、Caddy 代理和数据持久化 |
| **四家 DNS 服务商** | 真实账号权限、A/AAAA 更新、TXT 验证记录传播与精确清理 |
| **公网 HTTPS** | Let's Encrypt 测试/正式签发、自动续期、公网 80/443、路由器和防火墙 |
| **Linux / VPS Docker** | GHCR 拉取、host/bridge 实际端口、运行权限、重建升级和数据卷备份恢复 |
| **数据恢复** | 数据目录与 `dns/master.key`、证书私钥、数据库及已应用配置整体恢复 |

自动化测试中使用的模拟服务商、虚构域名和内置测试 CA **不能代替真实服务商及公网证书的验收**。

## 开发者本地测试

```bash
go test ./...
go test -race ./internal/...
go vet ./...

cd web
npm ci
npm test
npm run build
```

执行平台 smoke 前还需要准备相应 OS、构建后的 NetLite、真实标准 Caddy 或 Docker 环境；参见 [构建指南](BUILD.md) 与 [Docker 部署](DOCKER.md)。

## 历史测试与当前版本的区别

0.1.1–0.1.4 阶段曾测试过 ddns-go 桥接、令牌鉴权和带 DNS 插件的 Caddy。**那些历史成功记录仅适用于当时的架构**，不能继续当作当前版本的验证结果。当前应按以下文档验收：

- [飞牛 FPK 安装与验收](FPK.md)：统一网关和管理员登录态，不再使用 18780 管理 TCP 端口。
- [DNS 账号](DNS-ACCOUNTS.md)、[内置 DDNS](DDNS-INTEGRATION.md)：不再部署 ddns-go。
- [独立证书管理](CERTIFICATES.md)：证书签发/续期由 NetLite 管理，Caddy 加载证书文件。
- [Docker 部署](DOCKER.md)：host / bridge、GHCR 工作流、实际数据备份。
