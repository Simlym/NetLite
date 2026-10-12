> 历史验证记录：当前证书由 NetLite 管理，已移除 Caddy DNS 插件与证书自动化。当前架构见 CERTIFICATES.md，旧版验证结果不代表本次改造。

> 本文包含此前版本的验证记录。当前 DDNS 已内置，原 ddns-go 构建、二进制及测试结果不适用于当前引擎；当前验证要求见 DDNS-INTEGRATION.md。

# fnOS 官方规范核验

核验日期：2026-10-11（Asia/Shanghai）。直接通过 HTTPS 读取 developer.fnnas.com 的官方 HTML，不以第三方文章代替契约。

## 工具

- 官方文档：https://developer.fnnas.com/docs/cli/fnpack/
- 官方下载：https://static2.fnnas.com/fnpack/fnpack-1.2.3-windows-amd64
- 本地实际 --help：版本 1.2.3。
- create netlite 在 packaging/fnos 下生成 Native 官方模板；create 没有 --directory 选项，需要先进入父目录。
- build 使用应用目录为当前目录，或官方 --directory 参数；本工程进入新暂存目录后运行 build。

## 包结构

官方打包检查要求 manifest、config/privilege、config/resource、ICON.PNG、ICON_256.PNG、app/、cmd/、wizard/；声明 desktop_uidir 时相应目录必须存在。UI 配置位于 app/ui/config，图片为 app/ui/images/icon_64.png / icon_256.png。

本工程保留稳定 appname=netlite，原生 x86 ELF 使用 platform=x86；ARM 分包 platform=arm。不使用 platform=all 包含单架构二进制。没有填写未经实机验证的 os_min_version 或兼容上限。

生命周期：main start/stop/status；运行返回 0，停止返回 3。install_callback / upgrade_callback 创建仅应用可读的凭据环境文件；配置及卸载不会覆盖既有凭据和用户数据。TRIM_PKGVAR 持久化数据库、Caddy 证书和日志，TRIM_PKGETC 存放凭据，不硬编码卷号。

专用 package 用户，不声明共享目录、不注册系统路径链接、不获取 root、不修改系统 Web 服务。管理端口 18780 使用 service_port/checkport 交由 fnOS 检查；代理端口由规则控制且只接受非特权端口。主进程使用 /proc/PID/exe 检查 PID 身份，停止只发送自身服务 TERM，由后端关闭自己的 Caddy。

## 实机验收（仍需执行）

1. FPK 安装、桌面入口、包用户目录权限、18780 冲突提示。
2. 启动后两个进程、Admin 仅回环、从 UI 保存/校验/应用。
3. 停止后两个进程都退出，重启恢复 applied.json，数据库/凭据不丢失。
4. 应用升级保留 var / etc 与管理令牌，数据 schema 正常。
5. 停止服务后备份 var / etc，再测试卸载选项的数据保留行为。
6. Cloudflare A、AAAA、公网地址变化、失败重试、DNS-01 测试 CA 泛域名、实际 TLS 证书及自动续期。
7. 确认 NAS 系统管理网页在安装/启动/停止/升级/卸载后持续可用。

没有 fnOS 设备时，最多确认打包器校验和包内容，不能把本地 Windows 的运行验证当作以上实机结果。

Windows 打包补充：实测 fnpack 1.2.3 在 Windows 生成的归档中脚本及二进制默认 0666。scripts/normalize-fpk.py 使用 Python 3 将 cmd/* 与 bin/netlite、bin/caddy 改为 0755，并按实测官方契约重新计算 app.tgz 的 MD5 更新 manifest.checksum；输入 checksum 不符合契约时直接拒绝处理。构建脚本已自动执行并回读核验。该步骤只处理生成的包，不改动 NAS 文件。构建机另需 Python 3。

