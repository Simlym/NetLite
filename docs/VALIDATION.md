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
