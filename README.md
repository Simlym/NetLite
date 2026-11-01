# NetLite

**让网络管理轻一点。**

NetLite 是一款面向 **fnOS（飞牛 NAS）与 Linux 服务器**的轻量网络管理工具。在一个网页中管理 **DDNS、HTTPS 证书和反向代理**，不必手动维护 Caddyfile，也不必为每个服务重复配置证书。

例如，把 NAS 上的 Jellyfin、相册或其他 Web 服务通过自己的域名开放访问；家庭公网 IP 变化时，自动更新 DNS 解析。

> **项目状态：开发与测试阶段。** 自动构建和模拟测试已覆盖主要流程，但 **fnOS 实机安装与网关权限、ARM64 原生运行、真实 DNS 服务商及公网证书签发仍待验收**。请先在测试环境试用，并备份原有配置。
>
> **版本提醒（截至 2026-11-01）：** `main` 分支的应用清单为 **0.1.5（开发版）**；[Releases](https://github.com/Simlym/NetLite/releases) 中最新公开版本为 **v0.1.4（历史实现）**。两者的认证及证书架构不同，请勿直接把当前文档当作 v0.1.4 的使用说明。当前开发构建从 [CI 成功记录](https://github.com/Simlym/NetLite/actions/workflows/ci.yml) 下载。

![NetLite 管理界面](docs/preview.png)

[主要功能](#主要功能) · [下载安装](#下载安装) · [快速上手](#快速上手) · [使用限制](#使用限制) · [文档与反馈](#文档与反馈)

## 主要功能

| 功能 | 你可以做什么 |
| --- | --- |
| **反向代理** | 通过不同域名访问不同的 HTTP/HTTPS 服务；支持 WebSocket、端口配置、目标连通性测试 |
| **DDNS 动态域名** | 定时检查公网 IPv4 / IPv6，自动更新 A / AAAA 记录并查看运行历史 |
| **HTTPS 证书** | 使用 Let's Encrypt 申请与自动续期证书；支持 DNS-01、HTTP-01 和泛域名证书 |
| **DNS 账号共用** | 一次配置 DNS 服务商账号，同时用于 DDNS 和 DNS-01 验证 |

支持的 DNS 服务商：**Cloudflare、DNSPod.cn、腾讯云 DNSPod、阿里云 DNS**。也支持用于局域网/测试的内部 CA（客户端需自行信任）。

### 界面预览

| DDNS 任务 | DNS 账号 |
| --- | --- |
| ![DDNS 任务管理](docs/ddns-manager.png) | ![DNS 账号管理](docs/certificate-dns-account.png) |

![反向代理管理](docs/caddy-view.png)

## 下载安装

| 平台 | 安装方式 | 说明 |
| --- | --- | --- |
| **飞牛 fnOS** | [FPK 安装包](#飞牛-fnos) | 从飞牛桌面打开，设计上复用飞牛管理员登录态 |
| **Linux / VPS（Docker）** | [Docker Compose](#docker-compose) | 推荐 Linux host 网络；网页新增业务端口无需反复修改端口映射 |
| **Linux 原生** | [安装脚本](#linux-原生) | 作为 systemd 服务运行 |

提供 `amd64` 和 `arm64` 构建产物；Windows 目前主要用于开发，不提供普通用户安装包。

**在哪里下载？**

- **当前开发版：** 打开 [GitHub Actions · CI](https://github.com/Simlym/NetLite/actions/workflows/ci.yml)，选择 **`main` 分支且结果为 Success** 的一次运行，在 **Artifacts** 下载 `netlite-packages`，解压后选择对应架构的文件。Actions 产物通常只保留 **14 天**，下载可能需要登录 GitHub。
- **历史发布版：** [GitHub Releases](https://github.com/Simlym/NetLite/releases)。请核对版本和对应文档；**v0.1.4 不等于当前 0.1.5 开发实现**。
- 文件名及 SHA-256 校验方法见 [安装包说明](docs/BUILD.md)。目前 **CI 不会自动创建 GitHub Release**，请勿将构建成功误认为正式发布或实机验收完成。

### 飞牛 fnOS

1. 按上面的方法下载 `netlite-<版本>-amd64.fpk` 或 `netlite-<版本>-arm64.fpk`。
2. 在飞牛 **应用中心 → 手动安装** 中选择 FPK。
3. 使用**飞牛管理员账号**登录飞牛，从应用入口打开 NetLite。

FPK 使用 **fnOS 统一网关**作为管理入口，不单独开放管理 TCP 端口，也不接管飞牛系统 Web 服务。**统一网关兼容版本与 Unix Socket 权限仍需实机验证**，如打不开入口，请先查看 [飞牛安装与验收说明](docs/FPK.md)，不要自行修改系统 Web 服务。

### Docker Compose

适用于已安装 **Docker Engine + Compose v2** 的 Linux / VPS。下面采用 `host` 网络模式：

~~~bash
mkdir -p ~/netlite && cd ~/netlite
curl -fL https://raw.githubusercontent.com/Simlym/NetLite/main/compose.host.yaml -o compose.host.yaml
curl -fL https://raw.githubusercontent.com/Simlym/NetLite/main/deploy/docker.env.example -o .env

# 编辑 NETLITE_IMAGE，建议固定已经验证的版本/sha 标签，而不是一直使用 main
nano .env

docker compose -f compose.host.yaml pull
docker compose -f compose.host.yaml up -d --wait
docker compose -f compose.host.yaml ps
~~~

**VPS host 模式默认允许远程访问管理面板**：NetLite 监听 `0.0.0.0:18780`，无需 SSH 隧道。检查云安全组与防火墙后，打开 `http://你的服务器公网IP:18780`。

**首次连接是明文 HTTP。** 建议先在云安全组将 TCP 18780 的入站来源限制为自己的公网 IP，在可信网络完成初始化。随后在「HTTPS 证书」申请正式证书，再到「面板设置」启用 HTTPS；确认后使用 `https://管理域名:18780`，HTTP 不再接受管理操作。

如需仅本机访问，可以在 `.env` 中显式设置 `NETLITE_MANAGEMENT_BIND=127.0.0.1`，再使用 SSH 隧道或 VPN。**通用 bridge 模式仍默认仅在宿主机回环地址发布管理端口。**

首次打开面板时，在**服务器**运行下面的命令获取一次性初始化码，再按提示创建管理员：

~~~bash
cd ~/netlite
docker compose -f compose.host.yaml exec --user 10001:10001 netlite \
  /opt/netlite/bin/netlite info --data /data
~~~

镜像来自 `ghcr.io/simlym/netlite`，需确认所选标签已成功发布并可拉取。`host` 模式会占用宿主机上所配置的业务端口。首次使用 HTTP 管理入口时应限制访问来源并尽快切换 HTTPS。需要 Docker `ports` 映射时可选用 `bridge` 方案。见 [Docker 部署、备份与升级](docs/DOCKER.md)。

### Linux 原生

1. 从 [CI 成功记录](https://github.com/Simlym/NetLite/actions/workflows/ci.yml) 下载 `netlite-packages`。
2. 解压与你的设备架构一致的 `netlite-<版本>-linux-amd64.tar.gz` 或 `netlite-<版本>-linux-arm64.tar.gz`。
3. 在解压目录执行：

~~~bash
sudo bash install.sh
~~~

安装器会引导设置管理入口和管理员账号，注册 systemd 服务，安装后显示访问地址。目标机器**无需安装 Go 或 Node.js**。详见 [Linux 安装与账号恢复](docs/LINUX.md)。

## 快速上手

以“通过自己的域名访问 NAS 上的 Jellyfin”为例：

~~~text
https://jellyfin.example.com
    ↓ 域名解析到你的公网 IP（可由 DDNS 更新）
公网 TCP 443 → 路由器映射到 NAS TCP 8443
    ↓
NetLite HTTPS 反向代理（监听 8443）
    ↓
Jellyfin：http://192.168.1.10:8096
~~~

以上域名、IP 和端口均为示例。只需完成四件事：**配置 DNS（IP 动态时启用 DDNS）→ 申请 HTTPS 证书 → 创建代理并保存应用 → 从外网验证访问**。

**[查看完整的第一次配置教程](docs/QUICKSTART.md)**：包含 DNS-01 / HTTP-01 的区别、飞牛端口映射、配置示例与排障步骤。

## 使用限制

- **不提供内网穿透：** DDNS 只负责更新解析；如果家庭网络使用 CGNAT、没有可用的公网入站连接，仅靠 NetLite 无法从外网直连 NAS。
- **代理范围：** 目前支持 HTTP/HTTPS/WebSocket，不支持通用 TCP/UDP 转发。NetLite 管理独立的 Caddy，不修改或停止系统已有 Web 服务。
- **端口边界：** fnOS FPK 业务入口使用 **1024–65535**，可将公网 443 通过路由器转发至 NAS 的高位端口；Linux 部署在权限允许时可直接监听 80/443。
- **证书前提：** DNS-01 需要 DNS API 权限；HTTP-01 要求公网 TCP 80 可以转发到 NetLite 的专用验证端口 **18082**；内部 CA / Let's Encrypt 测试证书需要客户端额外信任。
- **备份与安全：** 数据库、DNS 加密主密钥和证书私钥必须作为一套数据备份。VPS host 模式默认监听所有 IPv4 接口；**初次 HTTP 配置期间限制访问来源、尽快切换 HTTPS**，也不要执行 `docker compose down -v` 删除数据卷。

## 文档与反馈

- [第一次配置与常见问题](docs/QUICKSTART.md) · [下载与构建](docs/BUILD.md)
- [飞牛 FPK](docs/FPK.md) · [Docker 部署](docs/DOCKER.md) · [Linux 部署](docs/LINUX.md)
- [DDNS](docs/DDNS-INTEGRATION.md) · [DNS 账号](docs/DNS-ACCOUNTS.md) · [HTTPS 证书](docs/CERTIFICATES.md)
- [API](docs/API.md) · [自动化测试与待验收项](docs/VALIDATION.md)

## 开发与贡献

后端使用 **Go + chi + SQLite**，前端为 **Vue 3 + TypeScript**，当前反向代理引擎为 **Caddy**。证书由 NetLite 申请、保存与续期，Caddy 负责加载证书并代理流量；Nginx 适配尚未实现。

欢迎在 [Issues](https://github.com/Simlym/NetLite/issues) 反馈安装问题或功能建议，注明系统、部署方式、版本及复现步骤，并对日志脱敏；**不要上传 DNS API 密钥、管理员初始化码、证书私钥或完整数据目录**。第三方依赖许可见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
