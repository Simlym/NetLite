# fnOS 登录态接入调研

文档核验日期：2026-10-13；实现及本地验收日期：2026-10-14（Asia/Shanghai）。直接通过 HTTPS 读取飞牛官方开发文档。统一网关接入已实现；尚未执行 fnOS 实机或公网验证。

## 结论

NetLite 部署为 FPK 后，可以通过飞牛官方的**统一网关**复用 NAS 登录态：用户已经登录飞牛时，从桌面入口进入 NetLite 不再输入应用密码或管理令牌。

飞牛包使用 `fnos-gateway` 认证模式，独立 Linux 部署继续使用 `local` 模式。统一网关方案适合当前常驻 Go 服务；不需要收集飞牛账号密码、读取系统账号数据库或自行修改系统 Web 服务配置。

这里的免登录指免去第二次应用登录，访问仍需有效的飞牛会话。独立端口不会因为安装成 FPK 自动获得飞牛身份。

官方依据：[统一网关](https://developer.fnnas.com/docs/core-concepts/gateway-registration/)、[应用入口](https://developer.fnnas.com/docs/core-concepts/app-entry/)。

## 当前项目状态

| 位置 | 当前行为 | 接入影响 |
| --- | --- | --- |
| `packaging/fnos/netlite/cmd/main` | `fnos-gateway` 模式，Socket 健康检查 | 不开放管理 TCP 端口 |
| `packaging/fnos/netlite/app/ui/config` | `/app/netlite/` + `netlite.sock`，管理员可见 | 后端也统一检查管理员身份 |
| `cmd/netlite/main.go` / `gateway.go` | 支持三种认证模式；网关只允许 Linux Unix Socket | 校验前缀，安全发布和清理 Socket |
| `internal/auth/gateway.go` | Unix 连接标记、平台身份校验、管理员检查及 CSRF | 缺失身份和普通用户请求均被拒绝 |
| `web/src/App.vue` / `application-path.ts` | 自动获取飞牛身份；API/下载适配应用前缀 | 会话失效不会回退到本地登录 |
| `web/vite.config.ts` | 静态资源使用相对路径 | 同一构建支持独立部署和网关部署 |
| `internal/panel` | 独立部署继续自管面板 HTTPS | 网关模式忽略旧面板 HTTPS 设置，相关 API 返回 403 |

## 推荐访问链路

```text
用户登录飞牛
  → 桌面入口 /app/netlite/
  → fnOS 统一网关验证 NAS 会话
  → /var/apps/netlite/target/netlite.sock
  → NetLite 校验网关身份和管理员权限
  → 管理面板与 API
```

统一网关把认证后的身份放在 `X-Trim-Userid`、`X-Trim-Isadmin` 和 `X-Trim-Username` 中。应用仍负责业务权限；NetLite 管理 DNS 凭据、证书和全局代理配置，当前只允许飞牛管理员访问，普通用户返回 403。

这些 Header 只能在隔离的网关监听入口上信任。FPK 的管理面板仅监听 Unix Socket；TCP 模式不能接受伪造 Header 作为身份。Unix Socket 的所有者、连接权限和目录权限需要按实机网关运行身份确定。

来源：[统一网关的身份传递与业务鉴权](https://developer.fnnas.com/docs/core-concepts/gateway-registration/)。

## FPK 入口配置建议

当前安装包启用以下入口配置：

```json
{
  ".url": {
    "netlite.Application": {
      "title": "NetLite",
      "icon": "images/icon_{0}.png",
      "type": "iframe",
      "protocol": "",
      "gatewayPrefix": "/app/netlite",
      "gatewaySocket": "netlite.sock",
      "url": "/app/netlite/",
      "allUsers": false,
      "control": {
        "accessPerm": "readonly"
      }
    }
  }
}
```

Socket 必须位于 `${TRIM_APPDEST}`，配置中只填写文件名。网关模式忽略入口的 `protocol` 和 `port`。采用 `iframe` 可在飞牛桌面内打开；需要独立标签页时可以使用 `url`。

生命周期脚本使用 `netlite gateway-health --gateway-socket ...` 检查 Socket，无需 curl。manifest 已移除 `service_port` 并设置 `checkport=false`。代理规则监听端口、独立 Caddy 及回环 Admin 保持原有机制。

## Socket 权限与升级

默认 Socket 为 0600，只允许 package 用户及 root 连接。应用在私有临时目录中创建 Socket、设置权限后再原子发布；拒绝覆盖普通文件、符号链接和活跃 Socket，重启可以恢复已确认失效的旧 Socket，停止只删除自己创建的 Socket。

如果实机网关由其他用户运行，需先核实它的专用用户组及目录访问权限；只有确认该组可信且 package 用户可以使用该组时，才在 `${TRIM_PKGETC}/gateway-socket.conf` 中配置：

```ini
mode=0660
group=<已经核实的数字 GID>
```

该文件按数据解析，不作为 shell 脚本执行。不能设置 0666 或把无关 NAS 用户加入可信组。Go 命令行也支持 `--gateway-socket-mode` 和 `--gateway-socket-group`。默认权限是否能让目标 fnOS 网关连接尚未实机验证；若不具备合适的用户组，需先确认官方支持的权限接入方式。

从 0.1.4 的令牌模式升级后，既有业务数据库、DNS 密钥、证书和代理快照保留；旧 `admin-token` 不再用于飞牛模式，新安装不生成本地管理员凭据。旧独立面板 HTTPS 设置不加载、不跳转，业务证书仍由 NetLite 管理。FPK 版本已更新为 0.1.5。

不支持统一网关的旧 fnOS 不提供自动认证回退；可以继续使用旧部署方案或独立本地认证部署。统一网关最低版本尚需设备核验，因此未填写未经验证的 `os_min_version`。

来源：[统一网关入口配置](https://developer.fnnas.com/docs/core-concepts/gateway-registration/)、[Manifest 端口声明](https://developer.fnnas.com/docs/core-concepts/manifest/)。

## 实现范围与关键边界

1. **认证层**：实现 `fnos-gateway` Provider，校验完整、合法的身份 Header，扩展 Principal 的 UID、管理员标记和认证来源。所有管理 API 统一检查管理员权限。身份缺失或异常时拒绝访问，不自动降级为匿名模式。
2. **监听与路由**：增加 Unix Socket 监听；外部页面、静态资源和 API 保持在 `/app/netlite/` 下，可在服务边界校验并剥离前缀后复用现有路由。Socket 启停清理不能误删其他路径。
3. **前端**：统一 API/下载路径解析，适配静态资源 base；支持 `fnos-gateway` 的身份查询并自动进入面板；飞牛会话失效时提示重新登录飞牛，不能显示应用令牌输入框。
4. **请求保护**：继续保护写操作，适配平台模式的 CSRF/Origin 检查。核验网关转发的 Host、Origin 和路径，不能为解决转发差异而关闭校验。应用自己的请求保护不得依赖必须先创建本地管理员账户的逻辑。
5. **面板设置**：平台模式不提供本地密码修改和独立面板 HTTPS 切换，显示当前飞牛身份和系统访问方式。退出应用不能假称已经注销飞牛会话。
6. **生命周期和打包**：保持 `package` 用户运行；确认当前 `umask 077` 下网关能够连接 Socket，采用最小必要权限；使用原有 FPK 构建脚本和 checksum/权限修正规则。
7. **兼容性**：独立 Linux 保留本地账号认证。旧 fnOS 若不支持统一网关，可另保留明确的本地认证部署方式，但网关鉴权失败不能触发无认证回退。

## CGI、开放 API 和版本信息

`index.cgi` 也会在执行前由系统校验登录态，但每个请求启动 CGI 进程，官方将其定位为轻量入口。当前 NetLite 有常驻服务和管理 API，优先选择统一网关。只用 CGI 把浏览器重定向到未保护的 TCP 服务，不能保证后续 API 获得飞牛身份。

来源：[index.cgi](https://developer.fnnas.com/docs/core-concepts/index-cgi/)。

开放 API 的 `TRIM_API_TOKEN` 是应用后端调用系统能力的凭据，与当前浏览器用户身份不同。基本登录态复用依据网关 Header，无需为此把 token 传给浏览器，也无需额外接入文件授权 Scope。以后接入文件选择、主题等 JS SDK 能力时，再按文档声明 `micro_app=true` 和实际需要的 Scope。

来源：[开放 API 调用方式](https://developer.fnnas.com/api/calling/)。

官方更新日志记录，2026-05-09 补充统一网关文档；2026-07-31 新增的开放 API 要求 fnOS 1.2.0401 及以上、飞牛 App 1.34.0 及以上。该开放 API 的版本门槛不能直接当作统一网关的最低版本；本次读取的网关章节没有明确最低系统版本，需用设备验证后再设置 `os_min_version`。

来源：[官方更新日志](https://developer.fnnas.com/docs/update-log/)。

## 后续验证要求

验证按 AGENTS.md 执行 `go test ./...`、`go vet ./...`、`scripts/smoke.ps1` 和前端 `npm.cmd run build`，再用 `scripts/build-fpk.ps1` 构建验证。Linux 的 `cmd/netlite` 测试覆盖真实 Socket、完整服务子进程、管理员权限、CSRF、前缀路由、静态资源、升级时旧 HTTPS 设置和 SIGTERM 清理；`go test -tags webembed ./cmd/netlite` 使用真实内嵌前端。

本地已通过：Go 全量测试及 Windows/Linux 目标 vet、前端 14 项测试与构建、Windows HTTP/HTTPS 代理 smoke、WSL Linux 中执行的普通构建及内嵌前端构建网关测试、Bash 生命周期脚本语法检查。amd64/arm64 的 0.1.5 FPK 均通过官方 fnpack 打包及 Unix 权限、`app.tgz` checksum 回读核验。ARM64 为交叉构建，未执行原生运行测试。

需要 fnOS 设备验证：已有管理员会话直接进入；未登录访问被拦截；普通用户无法访问管理 API；伪造 Header 无法绕过；静态资源/API/下载路径正确；退出、会话到期和权限变更后的行为；package 用户的 Socket 连接权限；安装、升级、重启和停止清理；系统管理网页持续可用。

如果承诺飞牛移动端或 FN Connect 访问，还需在对应入口分别验证。本次没有 fnOS 设备或真实 DNS 凭据，实机安装、移动端、FN Connect 及公网验证均未执行。
