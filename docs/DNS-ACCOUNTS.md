# 公共 DNS 账号

DNS 账号独立于 DDNS 任务。DDNS 更新 A/AAAA，证书 DNS-01 创建和清理 TXT；二者可以引用同一账号但独立运行，无需开启 DDNS 才能申请证书。账户 API、DDNS 和证书引用保护共用配置锁。

凭据库存储位于 internal/dnsaccount，维持原 dns_accounts 表和 ddns-go/master.key 的位置及 AES-GCM 格式，已有账号不搬迁或重新填写。数据库与 master.key 必须一起备份。证书只存 account_id，DDNS 也只存 account_id。

## 厂商适配

| 账号类型 | Caddy 模块 | 固定版本 | 原生配置字段 |
|---|---|---|---|
| Cloudflare | cloudflare | v0.2.4 | api_token |
| DNSPod.cn ID/Token | dnspod | v0.0.5-0.20260325061251-fb7cc31cc04c | api_token，ID,Token 合并 |
| 腾讯云 DNSPod | tencentcloud | v0.4.3 | secretid / secretkey |
| 阿里云 DNS | alidns | v1.0.29 | access_key_id / access_key_secret |

DNSPod 固定源码版本已切换为 api_token，不能照旧 README 使用 auth_token；腾讯云的 JSON 字段也不能照 Caddyfile 的 secret_id 命名。四种配置均以实际构建的 Caddy validate 校验。

Caddy DNS 插件与 ddns-go 的厂商实现独立。增加新厂商时必须分别检查两个引擎的支持和字段映射；UI 按已安装 Caddy 模块筛选账号。DNSPod Token 与腾讯云 SecretId/SecretKey 是两种账号类型，不能混用。

## 生命周期与边界

- 新证书默认使用账号引用，旧 Cloudflare 环境变量配置兼容，可在编辑中切换。
- 凭据仅在校验/加载/启动时注入内存，通过 stdin 或回环 Admin 传给 Caddy；applied.json 保存引用，重启重新解密。Caddy Admin 的运行配置包含凭据，仅绑定回环且不向管理页面透传。
- 账号删除检查任务、保存的证书、最近已应用代理模板。移除草稿引用后，仍需应用配置才能删除旧账号。
- 引用中的账号不能变更厂商。轮换密钥允许，DDNS 自动重载；证书需应用到代理。原始密钥不回显。
- 同一账号必须同时具备目标域名的 A/AAAA 与 TXT 权限才能承担两种用途；界面保存不代表授权有效。

## 验证

已运行完整 Go 测试、go vet、前端构建与 HTTP/HTTPS/重启恢复 smoke；四厂商真实 Caddy validate 配置检查；模拟回环加载验证运行载荷包含凭据而 checkpoint 只含引用；引用删除保护、轮换、旧配置兼容测试。

隔离浏览器实例使用虚构 DNSPod 凭据，DDNS 任务保持停用，证书保存引用同一账号，预览与配置校验通过。未执行真实 DNS 修改。fnOS 实机安装/升级、真实 DNS-01 签发及续期未验证。
