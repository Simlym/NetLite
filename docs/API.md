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
  "routes": [{"id":"app","domain":"app.example.com","port":18443,"scheme":"https","upstream":"http://127.0.0.1:8080","certificate_id":"wildcard","enabled":true}],
  "certificates": [{"id":"wildcard","domains":["*.example.com","example.com"],"mode":"dns","email":"you@example.com","provider":"cloudflare","token_env":"NETLITE_CF_TOKEN","staging":true}],
  "ddns": [{"id":"home","provider":"cloudflare","zone_id":"replace-zone-id","record_id":"replace-record-id","name":"app.example.com","type":"A","ip":"","token_env":"NETLITE_CF_TOKEN","interval_seconds":300,"enabled":false}]
}
~~~

配置不包含密钥。域名使用小写 ASCII（国际域名请用 Punycode），泛域名仅接受最左 *.，不提供任意引擎配置透传。默认私有管理端口不能作为代理端口；上游只接受无路径的 HTTP(S) URL。

证书模式 internal / acme / dns；当前 dns provider=cloudflare。staging=false 使用 Let's Encrypt 正式 CA，默认界面 staging=true。
