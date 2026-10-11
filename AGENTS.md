# NetLite 开发约定

业务模型在 internal/model，保持不包含 Caddy/Nginx 原生配置。通过 internal/proxy/ProxyEngine 接入代理引擎。

不得修改或停止 fnOS 系统 Web 服务；使用独立 Caddy 进程、独立数据目录与回环 Admin。FPK 以 package 用户运行。

完成涉及业务逻辑的修改后运行 go test ./... 和 go vet ./...；代理行为使用 scripts/smoke.ps1 验证。前端执行 npm.cmd run build。

FPK 使用 scripts/build-fpk.ps1；Windows fnpack 的 Unix 权限由 normalize-fpk.py 修正并验证 app.tgz checksum。不要将数据、凭据、工具下载或构建产物提交到 Git。

没有 fnOS 设备或真实 DNS 凭据时，明确注明实机/公网验证未执行。
