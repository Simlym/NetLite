# 飞牛 fnOS 安装包（FPK）

本文描述**当前版本**的 FPK 安装与安全边界。NetLite 使用 fnOS **统一网关**复用管理员登录态，不再要求独立应用令牌或占用 18780 管理 TCP 端口。

> 当前应用清单版本为 `0.1.5`（以 `packaging/fnos/netlite/manifest` 为准）。提供自动化打包和本地测试，**尚未完成真实 fnOS 设备上的安装、网关 Socket 权限、升级与卸载验收**。

## 用户安装

1. 打开 [GitHub Actions · CI](https://github.com/Simlym/NetLite/actions/workflows/ci.yml)，进入一次成功的运行，在 **Artifacts** 下载 `netlite-packages` 并解压。
2. 按 NAS 的 CPU 架构选择 `netlite-<版本>-amd64.fpk` 或 `netlite-<版本>-arm64.fpk`。
3. 在飞牛应用中心选择**手动安装**并导入 FPK。
4. 使用**飞牛管理员**登录系统，从桌面入口打开 NetLite。管理入口路径为 `/app/netlite/`。

FPK 依赖系统支持统一网关，但**最低兼容 fnOS 版本尚未经过设备验证**。构建文件默认保留 14 天；完整说明见 [构建与下载](BUILD.md)。

## 当前运行机制

| 项目 | 当前实现 |
| --- | --- |
| 管理身份 | `--auth-mode fnos-gateway`，拒绝非管理员与无效网关身份 |
| 管理入口 | `/app/netlite/` → 应用私有 Unix Socket `netlite.sock`，**不开放管理 TCP 端口** |
| 运行权限 | 独立 package 用户，不以 root 执行业务服务 |
| 代理引擎 | 本应用管理独立标准 Caddy，管理 API 仅监听 `127.0.0.1:20190` |
| 业务入口 | fnOS 仅允许 `1024–65535` 端口，不接管系统 Web 的 80 / 443 |
| 数据 | `TRIM_PKGVAR` 保存数据库、DNS 主密钥、证书、应用快照与日志 |
| 面板访问 | 飞牛网关负责入口登录；NetLite 的**业务 HTTPS 证书**仍独立申请和续期 |

公网访问仍需要公网入口、域名解析、路由器转发和防火墙许可。NetLite 不修改或停止 fnOS 的系统 Web 服务，也不更改系统信任库。

### 网关 Socket 权限

默认 Socket 权限是 `0600`，仅 package 用户及 root 可访问。**实际 fnOS 网关的身份和 Socket 连接权限仍需实机核实**。确认可信组后，才可在应用配置目录的 `gateway-socket.conf` 中使用 `mode=0660` 及相应数值 `group`；不能直接设置 `0666`。权限验证与身份安全说明见 [FNOS-AUTH.md](FNOS-AUTH.md)。

## 开发者：打包与脚本

项目使用官方 `fnpack 1.2.3`：

- `packaging/fnos/netlite/manifest` 定义应用版本、架构与入口，当前 `checkport=false`，不声明 18780 管理端口。
- `packaging/fnos/netlite/app/ui/config` 声明统一网关 `/app/netlite/` 与 `netlite.sock`。
- `packaging/fnos/netlite/cmd/main` 实现 `start/stop/status` 并通过 `gateway-health` 检查启动。
- 运行数据使用 `TRIM_PKGVAR`，可选的 Socket 权限配置位于 `TRIM_PKGETC`，不硬编码卷号。
- x86-64 与 ARM64 分包，分别使用 `platform=x86` 和 `platform=arm`，不把单架构二进制打包为 `platform=all`。
- Windows 打包脚本 `scripts/build-fpk.ps1` 调用 fnpack，`scripts/normalize-fpk.py` 修正 Unix 执行权限并验证 `app.tgz` 校验和。

目标 NAS 不需要自行编译 Go 或 Vue。具体构建要求和命令见 [BUILD.md](BUILD.md)。

## 仍需进行的 fnOS 实机验收

1. 实际设备上 FPK 安装、页面静态文件、桌面入口和 API 访问正常。
2. 已登录管理员可进入；未登录、普通用户与伪造身份 Header 被拦截。
3. 网关有权限连接 Unix Socket；停止、重启与升级能正确恢复 Socket 与 Caddy 进程。
4. 业务代理监听高位端口，端口冲突能正确报错，不影响系统管理页面。
5. 数据库、DNS 主密钥、证书、已应用代理配置在重启与升级后保留；核对卸载提示。
6. 使用真实服务商验证 DDNS、DNS-01 证书签发与续期及公网访问。

**归档校验和、模拟 ACME/DNS 测试与 Linux smoke 均不能代替以上实机测试。**

## 历史方案说明

旧版文档中的 `18780` + `service_port/checkport`、管理 `admin-token`、ddns-go 子进程、Caddy DNS 插件均**不是当前 FPK 方案**。请以 [统一网关实现](FNOS-AUTH.md)、[内置 DDNS](DDNS-INTEGRATION.md) 和 [独立证书管理](CERTIFICATES.md) 为准。
