# API

全部 /api 请求需要 Authorization: Bearer <admin-token>。/healthz 不需要鉴权，仅表示后端 HTTP 服务存活。

## 代理业务配置

| 方法 | 路径 | 行为 |
|---|---|---|
| GET | /api/config | 读取 SQLite 草稿和 revision |
| PUT | /api/config | 完整替换 routes/certificates，修订冲突 409，业务校验失败 400 |
| POST | /api/proxy/validate | 检查启用 HTTPS 的证书可用性并用真实引擎校验，失败 422 |
| POST | /api/proxy/apply | 应用当前草稿并保存业务快照，返回 applied_revision；未就绪或过期证书拒绝应用 |
| GET | /api/proxy/config | 当前草稿和已应用代理渲染结果；证书未就绪时 draft=null，draft_error 给出原因 |
| GET | /api/status | 代理运行状态 |

Config 为 revision、routes 和 certificates。Route 字段为 id/name/domain/port/scheme/upstream/certificate_id/enabled。
Certificate 字段为 id/domains/mode/email/staging/account_id/renew_before_days。mode 为 internal/acme（HTTP-01）/dns（DNS-01）。
DNS-01 必须引用公共账号，已移除 provider/token_env。非内部 CA 需要有效邮箱，泛域名只允许 DNS-01。

示例证书：

```json
{"id":"wildcard","domains":["*.example.com","example.com"],"mode":"dns","email":"admin@example.com","staging":true,"account_id":"dns-account","renew_before_days":0}
```

证书保存后自动申请和续期；代理规则只在 apply 后生效。renew_before_days=0 表示自动窗口，或填写 1..60。
后台证书重载与重启只使用 proxy-applied.json 的业务快照，不激活草稿。代理配置中只有证书文件路径，没有 DNS 密钥或签发策略。

## 证书管理

| 方法 | 路径 | 行为 |
|---|---|---|
| GET | /api/certificates/ | certificates 状态映射和 http_challenge_address |
| POST | /api/certificates/{id}/issue | 返回 202 表示排队；不存在 404；失败退避或成功冷却期 409 |
| GET | /api/certificates/{id}/certificate | PEM 公开证书链，尚未就绪 409 |
| GET | /api/certificates/root | PEM 内部 CA 公开根证书，不返回私钥 |

状态字段：id/status/working/not_after/renew_at/last_issued/next_attempt/attempts/last_error/reload_error。
status 为 pending/queued/issuing/renewing/ready/failed/expired。
详细签发、续期、验证和清理行为见 [CERTIFICATES.md](CERTIFICATES.md)。

## 公共 DNS 与 DDNS

- GET /api/dns/：accounts（仅元数据和 has_secret）、tasks（含 state）、engine=native。
- GET /api/dns/accounts：账号元数据。
- PUT /api/dns/accounts/{id}：id/name/provider/credential_id/secret；空 secret 保留原密钥，新增必填。不回显密钥。
- DELETE /api/dns/accounts/{id}：被任务、保存证书、已应用证书、签发或清理日志引用时 409。
- PUT /api/dns/tasks/{id}：id/name/account_id/ipv4_domains/ipv6_domains/ipv4_url/ipv6_url/interval_seconds/ttl/enabled；字段或记录冲突 400。
- DELETE /api/dns/tasks/{id}：停止任务并删除历史，不删除服务商记录。
- POST /api/dns/tasks/{id}/run：202 表示已排队检查，不表示更新成功。
- GET /api/dns/tasks/{id}/history：最近 100 条结果。
- GET /api/dns-capabilities：NetLite 公共 DNS 层支持的 certificate_providers，与 Caddy 模块无关。

provider 为 cloudflare/dnspod/tencentcloud/alidns。引用中的账号不能换厂商，同厂商可轮换密钥。
DDNS state 提供 running/checking/result_known、ipv4/ipv6、ipv4_result/ipv6_result、last_check/last_finished/last_request、provider_requests/update_requests、http_status/network_error 和 records。
success 表示服务商确认写入，未验证公网 DNS 传播；更新计数是尝试次数。新记录使用任务 TTL，现有记录保留 TTL/线路，Cloudflare 仅修改 content。
该组 API 不使用代理 revision，并发编辑以最后保存为准。
详见 [DDNS-INTEGRATION.md](DDNS-INTEGRATION.md)。
