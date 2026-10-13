# Linux 安装与面板访问

Linux 发布包包含 NetLite、独立标准 Caddy、安装脚本和内嵌 Vue 前端。
生产机无需安装 Go、Node.js。源码开发可用 `--web web/dist`；正式构建先构建前端，再 `go build -tags webembed ./cmd/netlite`。

## 安装

解压对应架构发布包后，执行 `sudo bash install.sh`。安装器会询问端口、网络/仅本机访问方式、管理员用户名和密码；密码留空时生成临时密码，在安装完成时显示，首次登录必须更换。
安装器创建 netlite 系统用户，将程序放在 `/opt/netlite/bin`、私有数据放在 `/var/lib/netlite`，注册并启动 systemd 服务。访问地址在启动检查通过后显示。安装器不自动修改防火墙或云安全组；网络访问必须允许管理端口。公网长期访问应启用 HTTPS。

独立运行默认回环监听：`./bin/netlite --data ./data --caddy "$PWD/bin/caddy"`。
未使用安装器初始化账号时，执行 `./bin/netlite info --data ./data` 获取一次性初始化码，在网页设置管理员。该码沿用 admin-token 文件，但 local 模式下它不能访问业务 API，管理员创建后不能再次初始化。

## 登录与恢复

密码使用 bcrypt 保存，不保留明文；浏览器使用 HttpOnly、SameSite=Strict Cookie，HTTPS 下启用 Secure。写接口校验 CSRF Token 和请求 Origin。
普通会话空闲 8 小时过期；勾选保持登录则空闲 7 天过期，最长 30 天。修改/重置密码注销旧会话。

- `sudo netlite info`：查看管理员及面板访问配置，不显示原密码。
- `sudo netlite reset-password`：从标准输入读取新密码。
- `sudo netlite reset-https`：关闭面板 HTTPS 设置；随后执行 `sudo systemctl restart netlite`。

交互重置密码时可避免终端回显：

```bash
read -r -s -p '新密码：' netlite_password
printf '\n'
printf '%s\n' "$netlite_password" | sudo netlite reset-password
unset netlite_password
```

CLI 默认数据目录 `/var/lib/netlite`，自定义目录附加 `--data`。CLI 的本机权限由数据目录 0700 和文件权限保护。

## 面板 HTTPS

进入「面板设置」选择已签发证书、填写该证书覆盖的具体域名，点击「检查并启用」。域名必须从访问者网络解析到服务器；服务端校验证书有效期、密钥和域名，不代表已验证外部 DNS、NAT 或防火墙。
前端显示测试地址；在新 HTTPS 地址登录后确认，两分钟内未确认自动恢复原设置。重启也恢复最近已确认配置。测试期间原 HTTP 入口临时保留。
HTTP 和 TLS 使用同一管理端口。确认后，HTTP 页面自动跳转至已配置域名，写接口拒绝 HTTP；取消跳转时 HTTP 请求直接拒绝。无需绑定 443 或另开端口，也不依赖业务 Caddy。
证书 ID 与签发策略被绑定；使用期间不能删除或修改该策略，须先更换/停用面板 HTTPS。续期成功后新 TLS 握手加载新证书，失败保留原证书并重试。内部 CA / ACME 测试 CA 需要浏览器额外信任。
关闭 HTTPS 会恢复 HTTP，需要重新登录。失效证书导致启动失败时，用本机 reset-https 恢复，不自动降级公开 HTTP。面板证书和私钥不会通过设置 API 导出。

## 升级已有部署

备份整个数据目录。使用 local 模式启动旧数据目录后，旧 admin-token 可用于一次初始化管理员；不能继续当业务 API 令牌使用。
已有 fnOS 包和旧集成工具通过显式 `--auth-mode token` 保持令牌兼容，令牌仍位于私有数据目录。Linux 新安装默认 local。自动化客户端暂需显式选择兼容模式，不能用 local 初始化码调用业务接口。
现有 Linux 安装升级应停服务、替换二进制，再安装新 service 文件，确认 `/etc/netlite/netlite.env` 中监听地址后启动；未配置时新模板默认监听网络。重复运行安装器会拒绝覆盖已有程序和账号。

## fnOS 预留

认证 Provider 与业务 API 分离；`--auth-mode fnos-gateway` 预留但当前拒绝启动，不读取或信任用户伪造的身份 Header。
待核验官方网关文档后实现私有 Unix Socket、管理员身份鉴权和路径前缀。FPK 当前继续使用 token，不关闭认证，不修改 fnOS 系统 Web 服务。

Linux 原生验证不等于 fnOS 实机验证或真实 DNS/公网 ACME 验证；后者需实际设备与凭据。
