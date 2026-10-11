# DDNS 引擎选型与分步集成

决策日期：2026-10-11。用户已选择复用 [jeessy2/ddns-go](https://github.com/jeessy2/ddns-go)，使用独立子进程；不再逐个自行实现厂商协议。初始适配版本固定为 **v6.17.7**，更新版本前重新验证配置和状态兼容性。

## 当前边界

0.1.2 已接入账号/任务 API、管理界面、AES-GCM 凭据存储和运行时任务调度。FPK 包含独立 ddns-go 与许可证。旧 Cloudflare Worker 保留，不自动迁移。已能启动、停用与重启检查；0.1.3 已完成同步结果、当前 IP、最后检查时间与服务商请求计数的结构化桥接。

缺少 --ddns-go 引擎路径时可保存账号与停用任务，不能启用；FPK 自动提供引擎路径。新任务使用 HTTP(S) IP 查询接口，固定 IP 与网卡选择尚未接入。

## 已核对的上游能力

- [main.go](https://github.com/jeessy2/ddns-go/blob/v6.17.7/main.go)：`-c` 指定配置，`-noweb` 关闭 Web，`-f` 指定整个进程的更新间隔。由 NetLite 管理普通子进程，不调用上游的系统服务安装或自更新命令。
- [config/config.go](https://github.com/jeessy2/ddns-go/blob/v6.17.7/config/config.go)：YAML 的 `dnsconf` 支持多个配置组；每组有名称、DNS 服务商凭据、IPv4/IPv6 域名列表、IP 获取方式和 TTL。文件配置缓存在内存；外部修改文件后需要由适配器重启子进程，不能假设热加载。
- [dns/index.go](https://github.com/jeessy2/ddns-go/blob/v6.17.7/dns/index.go)：包含 DNSPod、腾讯云、阿里云、Cloudflare 等实现。更新周期属于进程，不属于单个配置组；首版统一周期，后续不同周期可按周期分组管理进程。
- DNSPod 的 `dnspod`（ID/Token）与 `tencentcloud`（SecretId/SecretKey）是不同类型，账号表单必须区分，不能混用。
- [config/webhook.go](https://github.com/jeessy2/ddns-go/blob/v6.17.7/config/webhook.go)：Webhook 在有变化或满足失败条件时回调，不是每次检查的心跳。不能只凭“没有回调”判定同步成功，也不能把进程运行中当作 DNS 更新成功。
- 当前上游 Web 路由主要服务其自身页面，不视为稳定的管理 API。NetLite 不依赖页面抓取或模拟登录维护配置。
- [LICENSE](https://github.com/jeessy2/ddns-go/blob/v6.17.7/LICENSE)：MIT；随包保留上游版权与许可证。

## 分工

NetLite 保存与校验任务、管理账号、生成私有配置、管理子进程和展示状态。ddns-go 执行 IP 获取、DNS 服务商访问、记录查询及更新。Caddy 继续负责代理与证书，与 DDNS 的运行独立。

业务模型不包含上游 YAML。适配器通过单独的 DDNS 引擎接口生成配置、启动、停止、重载和报告状态。既有 Cloudflare 配置先保留，迁移确认后停止旧 Worker，避免两个引擎同时维护同一记录。

凭据配置只写入应用私有目录，Linux 文件权限 0600、目录 0700；Windows 需要限制 ACL。权限限制不等于加密，后续账号管理需明确静态加密和密钥存储方案。配置预览与日志须脱敏，API 不回显密钥。下载工具、凭据与生成配置不提交 Git。

## 实施顺序与验收

| 阶段 | 内容 | 验收 |
|---|---|---|
| 1，选型 | 固定版本、校验下载、明确配置和状态边界 | Windows 版本检查；Linux 包校验；记录无稳定 API、缓存和全局周期约束 |
| 2，引擎接入 | DDNS 引擎接口、私有配置生成、启停/重载、退出处理 | 使用隔离目录和模拟 DNS 接口验证；不访问真实账号；关闭 NetLite 后不遗留子进程 |
| 3，模型与界面 | 账号与任务分离；名称、多域名、A/AAAA、IP 来源、服务商专属表单 | 保存、重启恢复、旧 Cloudflare 配置迁移；凭据不回显 |
| 4，同步状态 | 日志脱敏、回调鉴权、最后检查/变更时间、立即同步 | 正确区分未检查、未变化、成功、失败；不把日志文本当稳定协议；必要时对上游做最小可维护状态桥接 |
| 5，FPK | 包含引擎和许可、package 用户、升级及停止管理 | 归档权限、checksum 和生命周期验证；fnOS 实机、真实 DNS 修改需单独验收 |

“指定 IP”不通过任意 shell 命令实现：在适配阶段选择受控的本地 IP 响应接口等方式，再做跨平台验证。立即同步也不能仅把“子进程已启动”报告为“DNS 同步成功”。

## 本地准备

```powershell
.\scripts\download-ddns-go.ps1 -OS windows -Arch amd64
& .\tools\ddns-go-v6.17.7-windows-amd64\ddns-go.exe -v
.\scripts\download-ddns-go.ps1 -OS linux -Arch amd64
```

脚本只下载官方固定版本、验证发布 SHA256 并保留许可证，不安装服务、不启动定时同步、不修改 DNS。归档和二进制在被忽略的 tools 目录中。

本次验证：固定版本源码及发布资产清单已核对，下载脚本语法检查通过。官方发布包下载未取得有效文件，Windows 二进制启动和 Linux 归档校验暂未验证；不计为下载验收通过。fnOS 实机与真实 DNS 同步亦未验证。

## 本地源码构建与阶段 2 进展

用户提供的 `E:\git\ddns-go` 是干净的 v6.17.7 checkout。构建脚本不会修改该仓库，版本或工作区不符合预期时停止并保留文件。

```powershell
.\scripts\build-ddns-go.ps1 -Source E:\git\ddns-go -OS windows -Arch amd64
& .\tools\ddns-go-windows-amd64.exe -v
.\scripts\build-ddns-go.ps1 -Source E:\git\ddns-go -OS linux -Arch amd64
go test ./...
go vet ./...
```

2026-10-11 已验证：Windows 输出版本 v6.17.7；Linux amd64 ELF 已构建；适配器 JSON 输出通过该版本的 config.Config / yaml.v3 原生解析，未执行 DNS 修改。

适配器支持 Cloudflare、DNSPod、腾讯云和阿里云字段映射，IPv4/IPv6 域名列表（含泛域名）及 HTTP(S) IP 来源；这只是配置映射，不代表对应服务商已完成真实联调。未知服务商拒绝生成配置，避免上游默认落到阿里云。

`ProcessEngine` 支持启停、替换与父上下文取消，每任务独立进程以支持不同间隔。单任务和多任务的内存开销需在 fnOS 上评估。Linux 私有目录/文件使用 0700/0600；Windows 使用当前账号的目录 ACL。上游原始输出不持久化或回显，仅接收版本化结构化事件。

进程测试使用隔离的子进程夹具，验证启停、替换、父取消、缺少可执行文件和“运行中不等于更新成功”；不是实际服务商联调。真实引擎目前仅做版本检查与配置类型兼容验证。下一阶段将适配器接入任务管理和界面后，再完成实际引擎隔离集成测试。

迁移注意：原 Cloudflare Worker 按 Record ID 更新且保留 TTL；ddns-go 按域名查询、可能创建记录，并有自己的 TTL 策略。必须提供迁移预览/确认和兼容测试，不能直接静默切换旧任务。

## 阶段 3 验证（2026-10-11）

- 账号/任务独立存储在 SQLite 表 dns_accounts/dns_tasks，业务字段在 internal/model/ddns_task.go。任务操作不改变代理配置 revision。
- 密钥加密存储，不回显；编辑留空保留密钥，账号被任务引用时禁止删除。数据库和 ddns-go/master.key 必须一起备份；缺失密钥时拒绝静默重置。
- 启停、重启检查和后台重载已接入真实引擎；进程错误显示与 DNS 结果分开。原始日志仍不回显。
- 服务测试覆盖密钥不明文存储、不回显、重开解密、引用删除保护、启停、手动请求、重复任务和新旧记录冲突。代理 smoke 回归通过。
- 浏览器使用虚构凭据、.invalid 演示域名及不会返回 IP 的本地查询接口，验证创建账号/任务、真实引擎启动、立即请求、停用。没有真实 DNS 更新；不代表服务商认证通过。
- 0.1.2 FPK 已包含 ELF 架构校验后的 ddns-go；normalize-fpk.py 核验其 0755 权限和 app.tgz checksum。fnOS 安装/升级及公网真实 DNS 验证未执行。


## 阶段 4：结构化状态（0.1.3）

构建脚本在固定版本 v6.17.7 上使用 Go build overlay，生成 v6.17.7-netlite.1。不修改 E:\git\ddns-go；厂商查询及更新仍由上游执行。新增代码仅在检查开始、IP 获取、服务商 HTTP 请求/响应和检查结束时输出 schema=1 事件。管理端丢弃普通日志，只接收经过校验的 IP、时间、结果枚举、HTTP 状态和计数，不保存请求 URL、头、正文或密钥。

必须使用 scripts/build-ddns-go.ps1 构建的版本才能显示这些状态。官方原版下载包没有桥接；运行中只能表示进程已启动，不能据此报告结果。当前桥接覆盖 NetLite 已开放的 Cloudflare、DNSPod、腾讯云、阿里云；新增厂商需补充请求动作分类和测试。

页面分别显示最近获取的 IPv4/IPv6、本轮查询及更新请求计数、最近检查/请求时间、未变化/成功/失败。更新请求只表示 HTTP 传输层尝试调用，不保证到达服务商。成功基于上游厂商实现返回的更新状态，不代表所有公网 DNS 缓存已刷新。多域名按地址族聚合，任一域名失败时该地址族显示失败；逐域名详情尚未接入。

每轮检查完成后，将结构化结果持久化到 SQLite dns_history；每个任务保留最近 100 次，包含时间、IP、地址族结果、请求计数和响应状态。重启后恢复最后一次完成结果，运行标志以当前进程为准。任务详情展示检查／更新历史；删除任务同时删除历史。不保存原始请求日志或凭据。未完成的检查不生成历史。

测试使用模拟 HTTP 传输和子进程事件，验证四厂商动作分类、IP 获取失败、未变化、成功/失败、计数、流式解析、异常事件拒绝、密钥不泄露及停用保留状态；不进行真实 DNS 修改。
