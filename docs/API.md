# API

全部 /api 请求需要 Authorization: Bearer <admin-token>。/healthz 不需要鉴权，仅表示后端 HTTP 服务存活。

| 方法 | 路径 | 行为 |
|---|---|---|
| GET | /api/config | 读取当前已保存草稿及 revision |
| PUT | /api/config | 完整替换配置；revision 必须匹配，否则 409；字段/业务校验失败 400 |
| POST | /api/proxy/validate | 用真实 Caddy 校验当前草稿，插件和凭据检查失败 422 |
| POST | /api/proxy/apply | 校验、加载、保存 applied.json；返回 applied_revision；证书异步签发 |
| GET | /api/status | Caddy 运行状态/模块、DDNS 最近执行结果 |
| POST | /api/ddns/{id}/run | 对已启用 DDNS 立即执行；结果中的 ok 标记真实同步是否成功 |

PUT 是全量配置更新；反代/证书仅在 apply 生效，DDNS 的 enabled 保存后生效。

示例请求：

~~~json
{
  "revision": 0,
  "routes": [{"id":"app","name":"家庭相册","domain":"app.example.com","port":18443,"scheme":"https","upstream":"http://127.0.0.1:8080","certificate_id":"wildcard","enabled":true}],
  "certificates": [{"id":"wildcard","domains":["*.example.com","example.com"],"mode":"dns","email":"you@example.com","provider":"cloudflare","token_env":"NETLITE_CF_TOKEN","staging":true}],
  "ddns": [{"id":"home","provider":"cloudflare","zone_id":"replace-zone-id","record_id":"replace-record-id","name":"app.example.com","type":"A","ip":"","token_env":"NETLITE_CF_TOKEN","interval_seconds":300,"enabled":false}]
}
~~~

配置不包含密钥。域名使用小写 ASCII（国际域名请用 Punycode），泛域名仅接受最左 *.，不提供任意引擎配置透传。默认私有管理端口不能作为代理端口；上游只接受无路径的 HTTP(S) URL。

证书模式 internal / acme / dns；DNS-01 新配置使用 account_id；原 provider=cloudflare/token_env 继续兼容。staging=false 使用 Let's Encrypt 正式 CA，默认界面 staging=true。

GET /api/proxy/config：鉴权后查看 Caddy 配置草稿及最近成功应用的配置；无已应用配置时 applied=null，凭据值隐藏，仅保留环境变量引用。

代理规则的 `name` 为可选名称 / 备注，仅用于管理展示，不参与代理匹配；旧配置缺省时为空，保存后持久化到 SQLite。

## ddns-go（需相同管理令牌）

- GET /api/ddns-go/：accounts（仅元数据与 has_secret）、tasks（含进程 state）、available、version。
- PUT /api/ddns-go/accounts/{id}：id/name/provider/credential_id/secret；secret 留空保留已有密钥，新增或更换服务商时必填。返回不含 secret。
- DELETE /api/ddns-go/accounts/{id}：账号被任务引用时 409。
- PUT /api/ddns-go/tasks/{id}：id/name/account_id/ipv4_domains/ipv6_domains/ipv4_url/ipv6_url/interval_seconds/ttl/enabled；非法值或记录冲突 400。缺少引擎时仅可保存停用任务。
- DELETE /api/ddns-go/tasks/{id}：停止进程并删除任务，不删除 DNS 服务商的记录。
- POST /api/ddns-go/tasks/{id}/run：启用任务重启检查，202 表示请求已提交，不表示 DNS 更新成功。

provider：cloudflare/dnspod/tencentcloud/alidns。state.running 仅指进程存活，result_known 在收到本轮完成事件后为 true。新旧任务重复记录的旧 /api/config 更新返回 409。该组 API 不使用代理 revision；同一账号/任务的并发编辑以最后保存为准。


state 提供 ipv4/ipv6、ipv4_result/ipv6_result、checking、last_check/last_finished/last_request、provider_requests/update_requests、http_status/network_error。请求计数属于最近一轮检查，接口请求包含查询，更新请求仅表示尝试提交；并不保证服务商已收到。结果值为 disabled/ip_failed/invalid_domain/unchanged/success/failed。success 表示上游厂商实现确认更新成功，未执行公网 DNS 传播验证。完成结果持久化到本地 SQLite，每任务保留最近 100 次；重启恢复最后完成结果，运行标志按当前进程计算。


## 公共 DNS 账号（0.1.4）

GET /api/dns/accounts 返回 {accounts:[元数据]}；PUT/DELETE /api/dns/accounts/{id} 与原 /api/ddns-go/accounts/{id} 使用同一账号记录。原 API 保留兼容，不复制凭据。所有接口需管理令牌。

GET /api/dns-capabilities 返回 certificate_providers，按独立 Caddy 二进制已安装的插件筛选，不代表真实服务商授权已验证。

Certificate 新字段 account_id。mode=dns 时，可使用 account_id，且 provider/token_env 必须为空；或保留原 Cloudflare provider/token_env。其他模式禁止 account_id。账号不存在/不能解密时保存返回 400。DELETE 账号被任务、草稿证书或已应用配置引用时返回 409；证书引用中的账号禁止改厂商，轮换同厂商密钥允许。

代理配置生成账号引用占位符 {netlite.dns.<account-id>.<provider>.<field>}，仅属于 NetLite 的配置模板。校验、加载和启动时在内存解析为厂商插件字段。请勿将模板直接交给普通 Caddy 执行。明文不写入已应用配置或配置预览；回环 Caddy Admin 内存中含运行所需凭据。

DDNS 检查历史：`GET /api/ddns-go/tasks/{id}/history` 返回 `{history: State[]}`，按完成顺序倒序，最多 100 条。每次完成的检查自动保存；删除任务清除历史。
