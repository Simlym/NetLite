# NetLite

轻量跨平台网络管理工具。Go + chi + SQLite 后端，Vue 3 + TypeScript 管理面板。
第一部署目标为 fnOS FPK，同时支持 Linux 原生运行。

GitHub Actions 自动构建 Linux 与飞牛的 amd64 / arm64 部署包。检查通过后，在 `CI` 运行的 Artifacts 中下载 `netlite-packages`；触发方式、包内容、版本管理与部署说明见 [自动构建与部署包](docs/BUILD.md)。

## 架构

- NetLite 独立管理证书：签发、私钥与证书存储、自动续期、失败退避和状态。
- ACME 协议使用 acmez v3.1.2；支持 Let's Encrypt 测试/正式 CA 的 HTTP-01、DNS-01。
- 内置 CA 用于本地测试，根证书可下载，不修改系统信任库。
- Caddy 只负责 HTTP/HTTPS/WebSocket 反向代理和加载证书文件，不签发或续期证书。
- 四家 DNS 服务商（Cloudflare、DNSPod.cn、腾讯云 DNSPod、阿里云）由公共 DNS 层实现，DDNS 与 DNS-01 共用账号及 API。
- ProxyEngine 使用业务规则、证书文件引用与已应用快照；后续 Nginx 可以复用证书服务。当前仅实现 Caddy。

详见 [证书管理](docs/CERTIFICATES.md)、[DDNS](docs/DDNS-INTEGRATION.md)、[API](docs/API.md)。

本次为开发阶段架构变更，不迁移旧 Caddy 证书或原 applied.json，开发验证请使用新的数据目录。

## Linux 构建与运行

构建机需要 Git、Go 1.25+（构建固定版本 Caddy 会使用其要求的 Go 工具链）、Node.js 22.12+。
生产机器仅需构建后的 NetLite、标准 Caddy 和静态前端，无需 Go、Node.js、ddns-go 或 Caddy DNS 插件。

```bash
git clone https://github.com/Simlym/NetLite.git
cd NetLite
npm --prefix web ci
npm --prefix web run build
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/netlite ./cmd/netlite
bash scripts/build-caddy.sh linux amd64
umask 077
./bin/netlite --data data-linux --web web/dist --caddy "$PWD/dist/caddy-linux-amd64"
```

浏览器打开 http://127.0.0.1:18780；另一终端执行 `./bin/netlite info --data data-linux` 获取初始化码并设置管理员账号。Linux 安装、账号恢复与面板 HTTPS 见 [Linux 部署](docs/LINUX.md)。
ARM64 将 Caddy 脚本第二个架构参数改为 arm64，在对应机器构建后端，或使用 GOOS/GOARCH 交叉构建。
可复用 deploy/netlite.service：程序放在 /opt/netlite/bin，正式包前端内嵌在程序内，数据放在 /var/lib/netlite，服务以 netlite 用户运行。

远程管理可使用 SSH 隧道：`ssh -N -L 18780:127.0.0.1:18780 user@server`。
Linux 管理默认 HTTP 加账号/会话，可从面板设置启用 HTTPS；初始 HTTP，保留在可信局域网或 SSH 隧道；Caddy Admin 固定使用回环地址。

## Windows 开发

需要 Go、Node.js 22.12+、PowerShell 7。

```powershell
.\scripts\build-caddy.ps1 -OS windows
.\scripts\dev.ps1
```

浏览器打开 http://127.0.0.1:18780，首次初始化码通过 `./bin/netlite info --data data` 查看。
`dev.ps1 -NoCaddy` 可开发 UI、DDNS 和证书管理，无法应用代理规则。
前端热更新：在 web 目录执行 npm.cmd run dev；Vite API 转发到 18780。

## 证书使用流程

1. DNS-01 先添加公共 DNS 账号，账号需能查询域名与 TXT、创建和删除 TXT。
2. 在 HTTPS 证书页面添加域名、验证方式和联系邮箱，默认使用测试 CA。
3. 保存后后台自动申请，界面显示排队、签发、到期、计划续期及错误。失败按 15 分钟起步退避，最长 8 小时。
4. 证书可用后，HTTPS 代理选择对应证书，保存并应用代理。
5. 后台提前续期并重载已应用代理，未应用草稿不受影响。失败保留原有证书。

DNS-01 支持泛域名，支持 CNAME 验证委派；账号必须有实际目标 zone 的权限。
HTTP-01 使用 NetLite 专用验证端口，默认 18082，公网 TCP 80 必须映射到该端口。
验证监听仅在检查期间启动，只返回匹配 Host/token 的验证内容。当前不实现 TLS-ALPN-01。

内部 CA 根证书与测试 CA 证书需客户端手动信任。下载仅包含公开证书，不导出私钥。
标准 Caddy 无需任何服务商插件；证书已由 NetLite 签发，应用成功不再意味着等待 Caddy 签发。

## 端口与数据

- 管理端口：18780；私有 Caddy Admin：127.0.0.1:20190。
- HTTP-01：18082，可通过 --acme-http-listen 修改。三个端口不能用于代理规则。
- 代理只允许 1024..65535 的高位端口；使用 DNS-01 时无需开放验证端口。
- 数据目录包含 netlite.db、admin-token、dns/master.key、certificates/ 和 proxy-applied.json，备份应保留整个目录。
- certificates/ 包含 ACME 账号私钥、内部 CA 私钥和不可变版本的证书/私钥。Linux 私有目录 0700、文件 0600，Windows 限制当前账号 ACL。
- 暂存的 TXT 验证记录会记入数据库，失败或重启后重试精确清理，不删除整个 TXT RRset。
- proxy-applied.json 保存业务配置，启动和续期重载均从此快照渲染；草稿单独保存在 SQLite。

不修改或停止 fnOS 系统 Web 服务，不接管系统证书或信任库。

## fnOS FPK

使用官方 fnpack 1.2.3，构建机另需 Python 3。

```powershell
.\scripts\build-caddy.ps1 -OS linux -Arch amd64
.\scripts\build-fpk.ps1 -Arch amd64
```

安装包仅包含 NetLite、标准 Caddy、预构建前端与许可说明。Windows fnpack 生成的可执行权限由 normalize-fpk.py 修正并核验 app.tgz 校验和。
启动脚本以 package 用户运行，使用应用私有数据目录。管理面板通过飞牛统一网关 `/app/netlite/` 访问，复用飞牛管理员登录态，不再要求独立管理令牌，也不监听管理 TCP 端口。目标 fnOS 必须支持统一网关；Socket 权限及实机验收见 [飞牛认证接入](docs/FNOS-AUTH.md)。
FPK 构建与 CI 不能代替 fnOS 实机安装、升级和权限验证。

## 验证

```bash
go test ./...
go test -race ./internal/...
go vet ./...
cd web
npm ci
npm test
npm run build
```

GitHub CI 执行模拟 ACME HTTP-01/DNS-01 完整流程、四厂商 TXT 与 DDNS 测试、状态持久化、续期与失败保留，使用标准 Caddy 验证配置及 Linux/Windows HTTP/HTTPS/重启 smoke。
Windows 集成脚本为 scripts/smoke.ps1；Linux 为 scripts/smoke-linux.py，需要 NETLITE_TEST_CADDY 指向真实 Caddy 和预先构建 bin/netlite。
未提供真实 DNS 凭据或 fnOS 设备时，公网签发、实际 DNS 传播和 fnOS 生命周期需现场验证。

## 目录

```text
cmd/netlite/          启动、API、鉴权与服务生命周期
internal/model/       引擎无关业务模型
internal/certificates/ 证书生命周期、ACME 与验证
internal/dnsprovider/  四厂商公共 DNS API
internal/dnsaccount/   公共加密凭据库
internal/dnstask/      DDNS 调度与历史
internal/proxy/        ProxyEngine 与 Caddy 代理适配
internal/store/        SQLite 与代理草稿修订
web/                   Vue 管理界面
scripts/               标准 Caddy、打包与集成验证
packaging/fnos/        fnOS 模板与应用脚本
```

后续：Nginx Adapter、Docker 镜像与 Compose、证书导入/吊销、日志轮转及 fnOS 实机验证。
