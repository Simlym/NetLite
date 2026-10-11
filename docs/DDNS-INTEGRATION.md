# 内置 DDNS 引擎

NetLite 在 Go 后端内实现 DDNS，不再构建、打包或启动 ddns-go。
实现参考 ddns-go v6.17.7 的服务商 API 与签名；许可证见 THIRD_PARTY_NOTICES.md。
本次处于开发阶段，不提供旧 ddns-go 目录、旧 Cloudflare 任务或 API 的迁移。
使用全新的应用数据目录进行开发验证。

## 模块

- internal/dnstask/task.go：任务输入与校验。
- internal/dnstask/engine.go：协程调度、IP 获取、立即检查与结果聚合。
- internal/dnstask/{cloudflare,dnspod,tencent,alidns}.go：四家服务商。
- internal/dnstask/signing.go：腾讯云 TC3 与阿里云 RPC 签名。
- internal/dnstask/service.go：账号、任务与 API。
- internal/dnsaccount：AES-GCM 凭据库，主密钥位于 data/dns/master.key。
- internal/dnstask/history.go：每个任务最近 100 条完成检查记录。

每个启用任务持有一个协程，最多四个任务同时检查；同一任务的检查不重叠。
首次启用立即执行，后续从上次完成时间起等待设定间隔（60..86400 秒）。
立即检查通过有界通道排队，不重启任何进程。重复请求最多保留一个待执行检查。
更改任务或凭据会取消旧检查并等待退出，再启动新任务。
单次 HTTP 请求超时 15 秒，整轮检查超时五分钟；停止任务取消网络请求。
失败会记录到状态及历史，下次按任务间隔再次检查。

## 更新行为

支持 A、AAAA、泛域名和多个域名，IP 来源为返回纯文本 IP 的 HTTP(S) 接口。
校验地址族，拒绝非法或过长响应；获取 IP 失败时不调用 DNS 服务商。
域名使用 ASCII 小写格式，国际化域名需填写 Punycode。

从最长域名后缀到最短后缀查询服务商，找到实际托管的 zone。
不按最后两段硬拆根域名，因此支持 example.co.uk 和委派子 zone。
只在服务商明确返回域名不存在时继续尝试，认证或网络错误不视为记录缺失。
账号需要查询托管域名、查询记录以及创建/更新 A/AAAA 的权限。

- 每次检查查询真实服务商状态，不只依赖本地 IP 缓存。
- 只有一条同名同类型记录时更新；多条记录返回错误，避免误改线路或负载均衡。
- 已停用记录不会自动启用。
- IP 未变化时不提交更新。
- 新记录使用任务 TTL 和默认线路（Cloudflare 默认 DNS only）。
- 现有记录保留 TTL、线路；Cloudflare PATCH 仅修改 content，保留 proxied 等属性。
- 任务 TTL 是新建记录的 TTL；更新任务 TTL 不修改现有记录的 TTL。
- 不自动删除 DNS 记录。删除或停用任务仅停止本机管理。

结果包括每个域名的状态，IPv4/IPv6 汇总以失败优先，接口请求数和更新尝试数分别统计。
DNS 服务商返回成功后才标记更新成功；接口错误正文和请求 URL 不写入状态，防止泄露密钥。
密钥仅在内存中解密，不再生成包含明文密钥的任务配置文件。

## API

所有接口位于 /api/dns，使用管理 Bearer Token：

- GET /：账号、任务、状态；engine=native。
- GET /accounts；PUT /accounts/{id}；DELETE /accounts/{id}。
- PUT /tasks/{id}；DELETE /tasks/{id}。
- POST /tasks/{id}/run：返回 202 表示已排队，不代表 DNS 已更新。
- GET /tasks/{id}/history：最近 100 条检查。

账号被任务、证书草稿或已应用配置引用时禁止删除或更换服务商。
编辑账号时空密钥表示保留原密钥。
代理草稿仍有独立 revision；DDNS 任务无需应用代理即可运行。

## 验证

```
go test ./...
go test -race ./internal/dnstask
go vet ./...
cd web
npm ci
npm test
npm run build
```

模拟服务商测试覆盖四家新增、更新、不变、多记录、身份不匹配与 API 拒绝；
覆盖真实字段保留、签名固定向量、子 zone、泛域名、IP 失败、任务取消和并发上限。
Windows 可用 scripts/smoke-ddns-status.ps1 验证真实后端的 IP 失败与零 DNS 请求。
fnOS 实机和真实服务商 API 尚需现场验证；模拟 API 不代替公网验证。
