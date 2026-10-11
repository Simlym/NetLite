# NetLite

轻量跨平台网络管理工具。当前为 0.1.0 MVP，第一目标为 fnOS 原生 FPK。

## 已实现

- Go + chi API，SQLite 持久化；Vue 3 + TypeScript 管理面板。
- 按域名、端口的 HTTP / HTTPS 反向代理；HTTP 和 HTTPS 上游、WebSocket（由 Caddy 实现）。支持编辑、删除、启停规则。
- 证书策略：内部 CA、Let's Encrypt ACME、Cloudflare DNS-01（包含泛域名）；自动签发及续期由独立 Caddy 负责。
- Cloudflare DDNS：A / AAAA，定时更新、立即更新；指定 IP 或自动发现公网 IP。核对记录身份后仅 PATCH content，保留 TTL / proxied 等字段。
- 管理令牌鉴权；SQLite 修订号防止过期页面覆盖新配置；反代/证书保存与应用分开。
- 独立 Caddy 子进程、专用管理端口、应用私有证书目录；退出时关闭自己的 Caddy，Caddy 意外退出时后端退出并报告失败。
- fnpack 官方模板、FPK 构建脚本、独立包用户、桌面入口及生命周期脚本。

业务配置不保存 Caddy 原生 JSON；模型在 internal/model，ProxyEngine 接口在 internal/proxy/engine.go，当前 Caddy Adapter 负责配置生成、校验、应用和状态。后续 Nginx Adapter 可以复用规则模型；Nginx 的证书签发/续期实现需要另行接入，不假设其与 Caddy 相同。

## Windows 本地启动

前置：Go 1.25+（构建当前 Caddy 使用 Go 1.27）、Node.js 22.12+、PowerShell 7。当前工程位于 E:\git\NetLite。

~~~powershell
cd E:\git\NetLite
# 带 Cloudflare 插件的 Windows Caddy；首次构建需要联网下载 Go 依赖
.\scripts\build-caddy.ps1 -OS windows
.\scripts\dev.ps1
~~~

浏览器打开 http://127.0.0.1:18780 。另一终端读取管理令牌：

~~~powershell
Get-Content E:\git\NetLite\data\admin-token
~~~

如只开发界面/API，可使用 .\scripts\dev.ps1 -NoCaddy（不能应用代理）。前端热更新：

~~~powershell
cd E:\git\NetLite\web
npm.cmd ci
npm.cmd run dev
~~~

Vite 默认仅绑定 127.0.0.1，API 转发到 18780。生产前端通过 Go 提供，无需在 NAS 安装 Node.js。

## 打包 fnOS FPK

构建脚本使用官方 fnpack 1.2.3。tools/fnpack.exe 为本次从官方地址下载的开发工具，tools 不入版本管理；在新机器可通过以下方式下载：

~~~powershell
New-Item -ItemType Directory tools -Force
Invoke-WebRequest https://static2.fnnas.com/fnpack/fnpack-1.2.3-windows-amd64 -OutFile tools/fnpack.exe
.\tools\fnpack.exe --help
.\scripts\build-caddy.ps1 -OS linux -Arch amd64
.\scripts\build-fpk.ps1 -Arch amd64
~~~

输出：dist/netlite-0.1.0-amd64.fpk。此包使用 platform=x86，包含 Linux amd64 Go 后端、Caddy（Cloudflare 插件）、预构建前端，不依赖 NAS 的 Go / Node / Docker。

ARM 可使用 -Arch arm64 分别构建 Caddy 和 FPK，但尚未完成 ARM 实机验证。脚本检查 Caddy 为正确架构的 Linux ELF，避免把 Windows .exe 打入 FPK。打包使用 dist 下新建的暂存目录，不改动源码模板。

安装步骤：

1. 在 fnOS 应用中心使用手动安装，选择 FPK。
2. 启动 NetLite，从桌面入口打开管理页面（NAS 地址:18780）。
3. 管理员通过 SSH 读取 /var/apps/netlite/var/admin-token，输入登录面板。
4. 先用 HTTP 高位端口反代一个内网服务；校验配置，再应用到代理。
5. 初次测试证书使用内部 CA 或 Let's Encrypt 测试环境；确认域名与验证方式后再切正式 CA。

服务以 fnOS 专用 package 用户运行。使用 TRIM_APPDEST / TRIM_PKGVAR / TRIM_PKGETC 等环境变量；不修改或停止 fnOS 的 nginx、Caddy、系统配置、系统证书目录或其他 Web 服务。反代只接受 1024..65535 高位端口，默认例子 18080 / 18443；占用端口会使校验/应用失败，需换空闲端口。

私有 Caddy Admin 是 127.0.0.1:20190，启动时若已被其他服务占用则拒绝启动。管理界面使用 18780；这两个端口不能作为代理规则端口。系统端口、现有其他应用端口仍需在实机确认。

## 配置 Cloudflare 凭据

令牌以环境变量引用，不存入 SQLite，不通过 API 传输、不显示在 UI。Cloudflare 令牌建议限制到所管理的 Zone；DDNS 需要 DNS 编辑权限，DNS-01 插件需要 Zone 读取和 DNS 编辑权限。插件的具体要求见官方仓库。

Windows：在启动 NetLite 的终端设置变量；Caddy 子进程继承变量。

~~~powershell
$env:NETLITE_CF_TOKEN='你的 Cloudflare API Token'
.\scripts\dev.ps1
~~~

fnOS：管理员编辑 /var/apps/netlite/etc/credentials.env（仅用于管理员控制的环境配置文件，不接受网页上传）：

~~~sh
NETLITE_CF_TOKEN='你的 Cloudflare API Token'
~~~

保存后重启应用。此文件首次安装自动创建，权限 600；数据目录默认 umask 077。不要把此文件提交到版本库。卸载脚本不会主动删除 var / etc，卸载前仍应备份：fnOS 卸载选项可能决定最终数据保留情况。

DDNS：在 Cloudflare 创建目标 A 或 AAAA 记录，取得 Zone ID、Record ID，填入记录域名、类型以及变量名 NETLITE_CF_TOKEN。留空 IP 时分别查询 https://api4.ipify.org / https://api6.ipify.org；IPv6 不可用时 AAAA 会显示错误并保持原记录。保存并启用后约每 10 秒检查调度，到设定间隔时同步；手动按钮与任务串行执行。

DNS-01：在证书页面选择 DNS-01，填写 *.example.com 和需要的根域名 example.com。根域名不被泛域名覆盖。应用前检查 dns.providers.cloudflare 模块和变量是否存在；缺少插件时返回可读错误。当前 DNS 插件注册仅支持 Cloudflare，其他插件需增加 provider 字段映射及校验，不能填写任意 Caddy JSON。

## HTTPS 和端口

内部 CA 只用于本地测试，不会自动修改系统信任库；客户端需手动信任或在测试请求中跳过验证。

ACME 默认测试 CA，测试证书不会被普通浏览器信任。正式 CA 通过证书策略中取消“测试 CA”启用。ACME 的 HTTP-01 / TLS-ALPN-01 验证需要公网 80 / 443 可达，当前 Caddy 全局验证端口分别为 18080 / 18443，必须映射到这些端口并保证正确的 DNS。普通高位 HTTPS 访问端口不能代替公网验证端口。若 NAS 系统已有 80/443 服务，优先 DNS-01；不接管系统端口。

使用 DNS-01 时，签发无需开放公网验证端口，但实际访问服务仍需要网络/路由端口映射。HTTPS 规则必须选择覆盖其域名的证书策略。同一端口不能同时配置 HTTP 和 HTTPS。上游地址仅支持 http(s)://host[:port]，不含路径、用户名或密码；HTTPS 上游启用证书验证，不提供跳过验证选项。

“应用成功”表示 Caddy 接受配置，不表示公网证书已签发。查看 caddy.log 的 obtain / renewal / error；当前 UI 没有证书到期清单或完整日志查看器。未应用的反代/证书草稿不会在重启时自动生效；重启恢复 applied.json。已启用 DDNS 依据已保存配置运行，不需要点击代理应用。

## 测试

~~~powershell
cd E:\git\NetLite
$env:NETLITE_TEST_CADDY=(Resolve-Path .\tools\caddy.exe).Path
go test ./...
go vet ./...
.\scripts\smoke.ps1
cd web
npm.cmd run build
npm.cmd audit --audit-level=high
~~~

单元测试：规则边界、泛域名覆盖、SQLite 持久化/修订冲突、Caddy JSON 及真实配置校验、占用 Admin 端口拒绝接管、Cloudflare 模拟 API 中保留字段/拒绝身份不匹配。真实 Caddy DNS 插件测试只校验配置，使用假令牌，不提交真实 DNS 变更。

smoke.ps1 在 19780 / 21190 / 19881 / 19081 / 19444 启动独立测试服务，验证鉴权、过期版本拒绝、HTTP/HTTPS 反代、内部 CA、已应用配置重启恢复及草稿持久化。临时数据保留在 data/smoke-* 便于查看日志。Windows 测试通过独立 Caddy Admin 停止自己的 Caddy，避免强杀后遗留子进程。

详见 docs/VALIDATION.md。FPK 构建成功不能代替 fnOS 实机安装测试；当前没有连接 fnOS 设备，也没有真实 Cloudflare 凭据，因此公网 DDNS、正式/测试 ACME DNS-01 签发、fnOS 安装/启动/升级/卸载仍需要现场验证。

## 目录

~~~text
cmd/netlite/         Go 启动、API、鉴权、进程生命周期
internal/model/      与引擎无关的业务模型及校验
internal/store/      SQLite 配置与 DDNS 状态
internal/proxy/      ProxyEngine + Caddy Adapter
internal/ddns/       Provider + Cloudflare + 调度
web/                Vue 3 + TypeScript
packaging/fnos/     官方原生模板衍生的 FPK 元数据和脚本
deploy/             Linux 服务示例；后续部署边界
scripts/            开发、构建 Caddy、打包、集成验证
docs/               官方规范核验、API、验证记录
dist/               构建二进制与 FPK（忽略）
data/               本地数据库、令牌、证书、日志（忽略）
~~~

## 后续

Linux：后端支持 CGO_ENABLED=0 的 Linux amd64/arm64 构建；deploy/netlite.service 为非特权 systemd 示例，路径需按安装位置调整。

Docker：下一阶段加入镜像、持久化卷和进程健康检查，使用同一 Go 后端 + 独立 Caddy 子进程；本版不交付已验证 Docker 镜像。

DSM SPK：下一阶段复用静态 Linux 构建，单独实现 DSM 生命周期、权限和安装包规范；本版不声称可安装 SPK。

待完善：凭据管理 UI、其他 DNS 服务商、证书到期状态、审计/日志轮转、自动化 fnOS 实机测试、用户登录与管理界面 HTTPS、Nginx Adapter。当前管理界面使用令牌和 HTTP，应保留在可信局域网或 SSH 隧道，不把 18780 / 20190 直接映射到公网。

## 官方依据

- [fnpack 官方文档与下载](https://developer.fnnas.com/docs/cli/fnpack/)
- [fnOS 应用框架](https://developer.fnnas.com/docs/core-concepts/framework/)
- [Manifest](https://developer.fnnas.com/docs/core-concepts/manifest/)、[包权限](https://developer.fnnas.com/docs/core-concepts/privilege/)、[应用入口](https://developer.fnnas.com/docs/core-concepts/app-entry/)
- [Caddy 自动 HTTPS / DNS-01](https://caddyserver.com/docs/automatic-https)
- [Caddy Cloudflare 插件](https://github.com/caddy-dns/cloudflare)
- [Cloudflare DNS Record API](https://developers.cloudflare.com/api/resources/dns/subresources/records/methods/edit/)

Windows 打包补充：实测 fnpack 1.2.3 在 Windows 生成的归档中脚本及二进制默认 0666。scripts/normalize-fpk.py 使用 Python 3 将 cmd/* 与 bin/netlite、bin/caddy 改为 0755，并按实测官方契约重新计算 app.tgz 的 MD5 更新 manifest.checksum；输入 checksum 不符合契约时直接拒绝处理。构建脚本已自动执行并回读核验。该步骤只处理生成的包，不改动 NAS 文件。构建机另需 Python 3。
