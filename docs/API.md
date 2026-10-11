> DDNS 使用 `/api/dns` 的内置引擎接口，任务与账号字段及执行语义见 [DDNS-INTEGRATION.md](DDNS-INTEGRATION.md)。旧 `/api/ddns/{id}/run` 与配置中的 `ddns` 字段已移除。

# API

全部 /api 请求需要 Authorization: Bearer <admin-token>。/healthz 不需要鉴权，仅表示后端 HTTP 服务存活。

| 方法 | 路径 | 行为 |
|---|---|---|
| GET | /api/config | 读取当前已保存草稿及 revision |
| PUT | /api/config | 完整替换配置；revision 必须匹配，否则 409；字段/业务校验失败 400 |
| POST | /api/proxy/validate | 用真实 Caddy 校验当前草稿，插件和凭据检查失败 422 |
| POST | /api/proxy/apply | 校验、加载、保存 applied.json；返回 applied_revision；证书异步签发 |
| GET | /api/status | Caddy 运行状态/模块 |
| GET | /api/proxy/config | 生成的草稿及已应用配置，凭据隐藏 |
| GET | /api/dns/ | 公共 DNS 账号、任务与内置引擎状态 |

PUT /api/config 是全量代理配置更新；反代/证书仅在 apply 后生效。
DDNS 通过 /api/dns/tasks 独立保存，启用后自动运行，无需应用代理。

示例请求：

~~~json
{
  "revision": 0,
  "routes": [{"id":"app","name":"家庭相册","domain":"app.example.com","port":18443,"scheme":"https","upstream":"http://127.0.0.1:8080","certificate_id":"wildcard","enabled":true}],
  "certificates": [{"id":"wildcard","domains":["*.example.com","example.com"],"mode":"dns","email":"you@example.com","provider":"cloudflare","token_env":"NETLITE_CF_TOKEN","staging":true}]
}
~~~

配置不包含密钥。域名使用小写 ASCII（国际域名请用 Punycode），泛域名仅接受最左 *.，不提供任意引擎配置透传。默认私有管理端口不能作为代理端口；上游只接受无路径的 HTTP(S) URL。

证书模式 internal / acme / dns；DNS-01 新配置使用 account_id；原 provider=cloudflare/token_env 继续兼容。staging=false 使用 Let's Encrypt 正式 CA，默认界面 staging=true。

GET /api/proxy/config：鉴权后查看 Caddy 配置草稿及最近成功应用的配置；无已应用配置时 applied=null，凭据值隐藏，仅保留环境变量引用。

代理规则的 `name` 为可选名称 / 备注，仅用于管理展示，不参与代理匹配；旧配置缺省时为空，保存后持久化到 SQLite。

## 内置 DDNS 与公共 DNS 账号

- GET /api/dns/：accounts（仅元数据与 has_secret）、tasks（含 state）、engine=native。
- GET /api/dns/accounts：账号元数据。
- PUT /api/dns/accounts/{id}：id/name/provider/credential_id/secret；secret 留空保留已有密钥，新增必填。返回不含 secret。
- DELETE /api/dns/accounts/{id}：被任务、证书草稿或已应用配置引用时返回 409。
- PUT /api/dns/tasks/{id}：id/name/account_id/ipv4_domains/ipv6_domains/ipv4_url/ipv6_url/interval_seconds/ttl/enabled；非法值或启用记录冲突返回 400。
- DELETE /api/dns/tasks/{id}：取消并等待任务退出，删除任务和检查历史，不删除服务商记录。
- POST /api/dns/tasks/{id}/run：启用任务排队检查，202 表示已安排，不代表 DNS 更新成功。
- GET /api/dns/tasks/{id}/history：返回最近 100 条完成结果。

provider 为 cloudflare/dnspod/tencentcloud/alidns。账号被任务或证书引用时不能更换服务商。
state.running 表示任务调度器运行；checking 表示正在检查；result_known 表示当前显示的结果已完成。
state 包含 ipv4/ipv6、ipv4_result/ipv6_result、last_check/last_finished/last_request、
provider_requests/update_requests、http_status/network_error 及 records（各域名结果）。
结果值为 disabled/ip_failed/unchanged/success/failed。success 表示服务商确认写入，未验证公网 DNS 传播。
接口请求包含查询与更新；更新计数表示写入尝试。现有记录保留 TTL 和线路，新建使用任务 TTL。
同名同类型多条记录、停用记录及身份不匹配均返回任务失败，避免误改。
该组 API 不使用代理 revision；同一账号/任务的并发编辑以最后保存为准。

GET /api/dns-capabilities 返回当前 Caddy 二进制的 certificate_providers。
DNS-01 证书通过 account_id 引用公共账号；密钥仅在运行时解析，不写入配置草稿或 applied.json。
详细调度与服务商行为见 [DDNS-INTEGRATION.md](DDNS-INTEGRATION.md)。
