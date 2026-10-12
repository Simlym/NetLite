# 公共 DNS 账号

DDNS 与 ACME DNS-01 共用 internal/dnsprovider 和 internal/dnsaccount，不依赖 Caddy 插件。
支持 Cloudflare、DNSPod.cn ID/Token、腾讯云 DNSPod SecretId/SecretKey、阿里云 AccessKey。
DNSPod Token 与腾讯云 SecretKey 是不同账号类型，不能混用。

密钥由 AES-GCM 加密存入 SQLite，API 不回显；编辑留空表示保留。
主密钥位于 dns/master.key，与数据库和整个应用数据目录一并备份。
账号被任务、证书草稿、已应用证书、正在进行的签发或待清理 TXT 日志引用时不能删除或换厂商。
同厂商密钥可轮换，DDNS 自动重载，后续签发/续期与清理读取当前密钥；代理不获取 DNS 密钥。

DDNS 需要域名查询和 A/AAAA 查询、创建、更新权限。
DNS-01 需要实际验证 zone 的域名查询、TXT 查询、创建及删除权限；使用 CNAME 委派时还需目标 zone 的权限。
/api/dns-capabilities 返回 NetLite 支持的厂商，不再检查 Caddy 模块。
详见 [证书管理](CERTIFICATES.md) 和 [DDNS](DDNS-INTEGRATION.md)。
