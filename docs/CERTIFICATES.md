# 独立证书管理

NetLite 管理证书，代理引擎只消费证书文件。协议层使用 acmez v3.1.2，未自行重写 ACME。
当前支持内部 CA、Let's Encrypt 测试/正式 CA 的 HTTP-01 和 DNS-01，不使用 Caddy 的证书自动化、PKI 或 DNS 插件。

## 策略与状态

业务策略：id、domains、mode（internal/acme/dns）、email、staging、account_id、renew_before_days。
DNS-01 使用公共 DNS 账号；非内部 CA 必须填写联系邮箱。泛域名只允许 DNS-01。
renew_before_days=0 表示自动选择：有效期的三分之一，最多提前 30 天；可配置 1..60 天。
策略保存后自动申请，也可手动排队。证书状态独立于代理草稿修订号。

证书服务每分钟检查保存策略与已应用策略的并集。因此从草稿删除证书并不会停止仍在运行的代理所需续期。
同一策略一次只排一个请求，单个工作协程执行签发，整轮期限 15 分钟；失败从 15 分钟指数退避，最长 8 小时，手动请求不能绕过失败退避。
成功签发后一分钟内拒绝重复手动申请。首次内部 CA 的根私钥持久化，后续叶证书由同一 CA 签发，不自动安装信任。

## 发布与代理

每次签发先校验私钥匹配、有效期及所有请求 SAN，再写入 material/<策略摘要>/<随机版本>/cert.pem 和 key.pem。
只有两个文件均已写入并同步后，才更新 SQLite 中的版本引用。
发布失败或续期失败保留旧版本。当前不自动清理旧版本，以免破坏运行中的代理或回滚引用。

代理解析 CertificateFiles（证书链与私钥的文件路径）。Caddy 配置只有 load_files、TLS 选择和代理规则，不含 issuer、automate、DNS 凭据或内部 CA。
代理 Apply 需要被启用 HTTPS 路由引用的证书就绪；证书续期之后只重载 proxy-applied.json 的业务快照，不激活 SQLite 草稿。
代理重载失败会记录在证书状态并每分钟重试。失败时 Caddy 继续使用原运行配置。
重启允许加载原有到期证书并让后台续期恢复；新应用过期证书会被管理 API 拒绝。

## 验证与清理

HTTP-01 使用专用监听（默认 0.0.0.0:18082），不复用管理端口或代理端口；公网 TCP 80 必须可达。
监听只在验证时打开，并校验 Host、GET 方法及 token；完成后关闭。TLS-ALPN-01 尚未实现。

DNS-01 复用 internal/dnsprovider 的 TXT API。解析 CNAME 后在实际目标 zone 创建记录，等待系统 DNS 解析器看到指定值，再通知 CA 验证。
权限需要涵盖实际目标 zone 的域名查询、TXT 查询/创建/删除。
现有同名 TXT 值保留；相同现存值作为借用记录，不取得删除权。
创建前持久化精确值意图，成功后保存记录 ID；清理前重新核对 ID、名称、类型和值。
已被外部修改的记录不会删除，多条相同值且无法确认归属时拒绝清理。
清理失败持久保留日志，阻止删除相关账号；空闲时及重启后重试。

## 私钥与 API

数据目录必须整体备份。包含内部 CA、ACME 账号与证书私钥；主密钥 dns/master.key 也必须与数据库一起备份。
私钥只写入应用私有文件，不通过 API 导出，也不注入代理 JSON；代理只获取路径。

所有接口在管理令牌鉴权之后：

- GET /api/certificates/：各策略状态与验证监听地址。
- POST /api/certificates/{id}/issue：返回 202 表示已排队，不表示签发成功。
- GET /api/certificates/{id}/certificate：下载公开证书链。
- GET /api/certificates/root：下载内部 CA 的公开根证书。

状态包含 status、working、not_after、renew_at、last_issued、next_attempt、attempts、last_error、reload_error。
错误信息不回显原始 DNS 服务商响应或密钥。

## 验证范围

模拟 ACME 测试执行注册、订单、挑战、CSR 签名及证书下载，覆盖账号复用、HTTP-01 和根域名+泛域名 DNS-01，同时保留其他 TXT 值。
四厂商模拟 API 检查新增、借用、修改保护和精确删除。证书测试检查持久化、续期、失败保留、错误材料拒绝和公开导出。
标准 Caddy 集成验证可信 HTTPS、重启恢复、草稿隔离，以及续期后仅重载已应用规则。
真实公网 DNS 传播、Let's Encrypt 签发、fnOS 实机权限和 FPK 安装仍需现场验证。
