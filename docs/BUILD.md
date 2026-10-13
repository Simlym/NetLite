# 自动构建与部署包

GitHub Actions 的 `CI` 工作流在推送 `main` / `codex/**` 分支、创建或更新 PR、推送 `v*` 标签时运行，也可在 Actions 页面通过 `Run workflow` 手动运行。

Go 测试、race 检查、vet、前端测试与构建、Linux / Windows 代理 smoke 全部通过后，调用 `.github/workflows/build.yml` 构建部署包。不需要 DNS 凭据或额外的 GitHub Secrets。当前只上传 Actions Artifacts，不自动创建 Release 或部署到服务器。

## 产物

打开对应 Actions 运行，下载 `netlite-packages`，解压外层 ZIP 后得到：

| 文件 | 用途 |
| --- | --- |
| `netlite-<版本>-linux-amd64.tar.gz` | Linux x86-64 |
| `netlite-<版本>-linux-arm64.tar.gz` | Linux ARM64 |
| `netlite-<版本>-amd64.fpk` | 飞牛 x86-64 安装包 |
| `netlite-<版本>-arm64.fpk` | 飞牛 ARM64 安装包，需目标 fnOS 支持 |
| `SHA256SUMS` | 四个文件的 SHA-256 校验值 |

也可单独下载 `netlite-linux-amd64` / `netlite-linux-arm64` / `netlite-fnos-amd64` / `netlite-fnos-arm64`。部署包保留 14 天，前端中间产物 `netlite-web` 保留 7 天，长期保存请下载或手动上传至 Release。

在四个产物所在目录运行 `sha256sum --check SHA256SUMS` 校验。版本统一读取 `packaging/fnos/netlite/manifest` 的 `version`；发布标签必须严格对应，例如 manifest 为 `0.1.5` 时使用 `v0.1.5`。更新版本只需修改 manifest。

Linux 包含 `bin/netlite`、标准 `bin/caddy`、`web/` 静态前端（同时内嵌在程序内）、`install.sh`、`deploy/netlite.service`、许可说明和 `BUILD-INFO`（版本、架构、源码提交）。飞牛包复用同架构 Linux 包中的二进制和前端，并调用 `scripts/build-fpk.ps1`；固定官方 fnpack 1.2.3，下载后校验 SHA-256，打包后自动修正 Unix 权限并回读核验 `app.tgz` checksum。

## Linux 运行

将对应架构的 `.tar.gz` 解压到空目录，在解压后的目录运行：

```bash
umask 077
./bin/netlite --listen 127.0.0.1:18780 --data ./data --web ./web --caddy "$PWD/bin/caddy"
```

执行 `./bin/netlite info --data ./data` 获取初始化码，打开 `http://127.0.0.1:18780` 创建管理员。正式包建议直接执行 `sudo bash install.sh`，安装流程与 HTTPS 切换见 [Linux 部署](LINUX.md)。远程管理可用 SSH 隧道：`ssh -N -L 18780:127.0.0.1:18780 user@server`。程序使用独立 Caddy 进程与回环 Admin，不需要目标机器安装 Go 或 Node.js。

使用 systemd 时，将包内 `bin/`、`web/` 放到 `/opt/netlite/`，准备 `netlite` 用户及其拥有的 `/var/lib/netlite`（权限 0700），将 `deploy/netlite.service` 复制到 `/etc/systemd/system/`，再执行 `systemctl daemon-reload` 和 `systemctl enable --now netlite`。不要将源代码目录或数据目录一起打包。

## 飞牛安装

在 fnOS 应用中心手动安装与设备架构对应的 `.fpk`。服务以独立 package 用户运行，使用统一网关 `/app/netlite/` 复用飞牛管理员登录态；不开放管理 TCP 端口。目标系统须支持统一网关，Socket 权限与验收见 [飞牛认证接入](FNOS-AUTH.md)。不修改或停止 fnOS 系统 Web 服务。

CI 中 amd64 Linux 包执行实际二进制 smoke；ARM64 为交叉构建和包结构检查，未执行 ARM64 原生运行验证。FPK 的打包校验不代表 fnOS 实机安装、升级、卸载和权限验证通过；没有设备及真实 DNS 凭据时，这些实机与公网 ACME / DNS 验证未执行。

## 本地构建

Linux 构建机需要 Git、Go 1.25+、Node.js 22.12+，标准 Caddy 构建会使用其要求的 Go 工具链：

```bash
bash scripts/build-caddy.sh linux amd64
bash scripts/build-linux.sh amd64
# 已构建前端时可追加 --skip-web-build；ARM64 将 amd64 改为 arm64。
```

Windows 的 FPK 构建需要 PowerShell 7、Go、Node.js、Python 3 及官方 Windows fnpack，按项目 README 使用 `scripts/build-caddy.ps1` 和 `scripts/build-fpk.ps1`。`-NetLiteBinary` 可传入已构建的同架构 Linux NetLite，`-SkipWebBuild` 复用现有 `web/dist`，否则脚本仍会自行编译。

## 后续 Docker 扩展

`ci.yml` 负责检查并调用可复用的 `build.yml`，其中 `linux`、`fnos`、`collect` 分别负责编译、平台打包、汇总。后续可在 `build.yml` 添加依赖 `linux` 的 Docker job，复用同架构包中的 `bin/` 和 `web/`；镜像推送应限定在可信分支或标签，并仅对推送 job 授予 `packages: write`。当前尚未构建或推送 Docker 镜像。
