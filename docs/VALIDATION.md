> 历史验证记录：当前证书由 NetLite 管理，已移除 Caddy DNS 插件与证书自动化。当前架构见 CERTIFICATES.md，旧版验证结果不代表本次改造。

> 本文包含此前版本的验证记录。当前 DDNS 已内置，原 ddns-go 构建、二进制及测试结果不适用于当前引擎；当前验证要求见 DDNS-INTEGRATION.md。

# 本地验证记录

DDNS 引擎适配阶段：本地 ddns-go v6.17.7 已构建 Windows/Linux amd64，Windows 版本检查通过；生成配置通过上游原生 YAML 解析验证。internal/ddnsgo 的配置校验与进程夹具测试、go test ./...、go vet ./... 通过。尚未接入运行时调度和界面，未进行真实 DNS 更新或 fnOS 实机验证。见 DDNS-INTEGRATION.md。

验证日期：2026-10-11，Windows amd64。

| 项目 | 结果 |
|---|---|
| Go 后端与 SQLite | go test ./... 通过 |
| 静态检查 | go vet ./... 通过 |
| 真实 Caddy JSON | 内部 CA、HTTP/HTTPS、Cloudflare DNS-01 配置校验通过 |
| 插件 | Windows / Linux Caddy 2.11.7 + caddy-dns/cloudflare 0.2.4 |
| 集成验证 | 令牌鉴权、revision 冲突、HTTP、内部 CA HTTPS、草稿保存和已应用配置重启恢复通过 |
| 浏览器 | 登录、代理保存/校验/应用、证书策略保存、停用 DDNS 保存通过 |
| 前端 | TypeScript / Vite 构建通过；npm audit 0 vulnerabilities |
| Linux 构建 | CGO_ENABLED=0 的 amd64 后端、Cloudflare 插件 Caddy 已构建 |
| 生命周期 | 所有 cmd 脚本 Bash 语法检查通过；高位端口、包用户、数据私有 |
| FPK | 官方 fnpack 1.2.3 构建成功，Windows 归档权限补正并回读核验 checksum |

Cloudflare 行为通过模拟 HTTP API 测试：只 PATCH content，保留其他属性，错误域名/类型不会写入。DNS-01 真实 Caddy 校验使用合法格式假令牌，只检查配置，不进行实际 DNS 修改或签发。

未验证：fnOS 实机安装、目录权限、启动/停止/升级/卸载；真实 Cloudflare DDNS、ACME DNS-01 泛域名签发和续期；ARM、Docker、DSM。安装包构建成功不等同于这些项通过。

详见 FPK.md 的实机验收列表。界面截图：preview.png。

0.1.1 UI 验证：添加/编辑弹窗、保存自动关闭并更新列表、取消、Escape、首次输入框焦点、删除确认、证书与 DDNS 弹窗、Caddy 草稿/已应用查看均已浏览器验证。配置查看脱敏单元测试与 Go 测试、静态检查、代理集成测试、前端构建通过。新版 FPK 已生成；仍未进行 fnOS 实机升级验证。

代理规则表格验证：访问域名加粗、访问端口高亮、协议弱化、目标服务/状态/操作分列；桌面与 390px 窄屏横向滚动已验证，添加与编辑保留弹窗。前端类型检查及构建通过，FPK 已重新打包并验证归档权限和 checksum。

协议列优化：访问域名、访问协议、访问端口、目标服务地址、状态、操作六列；HTTP/HTTPS 独立标签，目标显示完整 URL。前端构建和浏览器显示验证通过，FPK 已更新。

对称布局验证：弹窗访问端/目标端上下排列，每组协议、地址、端口；表格按相同顺序左右分组。浏览器已验证保存、重新编辑、目标 HTTPS 与 IPv6、访问 HTTPS 时显示证书选择、取消及 390px 弹窗。npm --prefix web test 的 4 项地址转换测试通过，覆盖 IPv6、默认端口与非法地址/端口；前端类型检查、构建及 FPK 归档核验通过。

代理名称验证：name 可选，表格首列和添加/编辑弹窗显示名称 / 备注；浏览器验证中文名称保存、刷新、重新编辑和旧规则空名称。SQLite 重启持久化及缺少 name 的旧配置兼容测试通过；go test ./...、go vet ./...、前端构建、真实 Caddy 集成验证通过，FPK 已重新生成并核验。

0.1.2 DDNS：账号与任务界面/API接入；加密、重开解密、不回显、空密钥保留、账号引用、启停、立即请求、新旧任务冲突测试通过；前端构建与代理 smoke 回归通过。浏览器验证真实 ddns-go 进程的启动、重启检查及停用，使用虚构凭据和无 IP 的本地接口；真实服务商、公网更新及 fnOS 安装未验收。


0.1.3 DDNS 状态与界面：四厂商桥接模拟测试、事件校验与脱敏、子进程状态管道、IP 获取失败及停止保留结果测试通过；go test ./...、go vet ./...、npm.cmd run build 和代理 smoke 均通过。scripts/smoke-ddns-status.ps1 使用真实桥接引擎、虚构账号和不可返回 IP 的本地接口，验证检查时间、IP 获取失败、接口/更新请求均为零，不修改真实 DNS。

列表精简为任务、域名、公网 IP、同步结果、最近检查、操作六列；账号折叠管理，次数/完整时间/响应码/启停/删除放入详情。浏览器使用模拟数据验证 A/AAAA、多域名、详情弹窗和账号展开；窄屏可横向滚动。docs/ddns-manager.png 和 docs/ddns-task-detail.png 为模拟数据截图。

fnOS 实机安装、升级及真实服务商更新/公网传播验证未执行。


0.1.4 公共 DNS 账号：既有表和主密钥兼容、证书引用与旧环境变量配置、草稿及已应用引用保护、轮换、配置预览/持久化不含密钥、运行时注入测试通过。完整 go test ./...、go vet ./...、前端构建、代理 smoke 通过。实际构建的四插件 Windows Caddy 对 Cloudflare/DNSPod/腾讯云/阿里云配置 validate 均通过；运行载荷用模拟 Admin 接收，没有真实签发或 DNS 请求。

隔离浏览器使用虚构账号、停用的 DDNS 任务；证书选择已有 DNSPod 账号后保存 account_id=shared，GET 配置仅含引用，预览不含 Token，真实 Caddy 配置校验通过。docs/certificate-dns-account.png 为该测试截图。fnOS 实机、真实 DNS-01 签发与续期未验证。

