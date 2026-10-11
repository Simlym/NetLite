# 本地验证记录

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
