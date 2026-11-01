# Docker Compose 部署（Ubuntu / 通用 Linux）

NetLite 镜像包含内嵌前端的 Go 后端和标准 Caddy；DDNS、公共 DNS API 与证书生命周期均由 NetLite 内置管理。由后端管理自己的子进程，不连接宿主机已有 Caddy，也不修改系统 Web 服务。运行时无需 Go、Node.js 或 Python。

支持构建 `linux/amd64` 和 `linux/arm64` 镜像。Linux VPS 推荐 host 网络，可在网页直接新增入口端口；`compose.yaml` 仍保留 bridge 配置兼容已有部署。FPK 构建路径保持独立。

## Linux VPS 推荐部署

首次部署需要 Docker Engine 和 Compose v2。先准备部署文件，并确认目标镜像已成功发布：

```bash
mkdir -p ~/netlite && cd ~/netlite
curl -fL https://raw.githubusercontent.com/Simlym/NetLite/main/compose.host.yaml -o compose.host.yaml
curl -fL https://raw.githubusercontent.com/Simlym/NetLite/main/deploy/docker.env.example -o .env
# 编辑 .env 的 NETLITE_IMAGE，推荐固定已经验收的版本或 sha 标签
```


```bash
docker compose -f compose.host.yaml up -d --wait
```

此方式不需要 `ports` 映射。网页 HTTPS 默认端口 443、HTTP 默认 80，其他业务入口端口直接在网页配置。

**VPS host 模式的管理面板默认监听 `0.0.0.0:18780`**（所有 IPv4 接口）。在云安全组和系统防火墙允许访问后，可以直接打开 `http://服务器公网IP:18780`。**首次管理连接为明文 HTTP**，建议在云安全组中先将 TCP 18780 的入站来源限制为自己的 IP；NetLite 不会自动修改安全组或防火墙。

在服务器获取首次初始化码，并在网页创建管理员：

```bash
docker compose -f compose.host.yaml exec --user 10001:10001 netlite /opt/netlite/bin/netlite info --data /data
```

然后在「DNS 账号」设置必要凭据，在「HTTPS 证书」中申请覆盖管理域名的正式证书，再进入「面板设置」检查并启用管理 HTTPS。新地址是 `https://管理域名:18780`，需要在新地址登录并确认；两分钟内未确认将自动回滚。成功确认后 HTTP 不再接受管理操作，可选 GET 自动跳转。**管理 HTTPS 仍使用 18780 端口，不会自动改为 443**。

若希望管理面板仅供本机访问，在 `.env` 显式指定 `NETLITE_MANAGEMENT_BIND=127.0.0.1`，再通过 SSH 或 VPN 访问。宿主机业务端口（如 443）必须空闲，安全组和防火墙仍需允许业务访问；NetLite 不会停止已有服务来腾出端口。

例如 `xq.teanap.top → http://127.0.0.1:8080`：在网页选择证书，填写 HTTPS 443、目标 HTTP / 127.0.0.1 / 8080，点击“测试目标连接”后“保存并应用”。host 模式的 127.0.0.1 就是 VPS 宿主机。证书需先就绪；公共证书建议 DNS-01。HTTP-01 仍需要公网 80 到独立 18082 验证监听器的转发，业务 80 不会自动转交验证请求。

host 模板以 root 启动短暂的 `setpriv` 引导，授予降权所需的 SETUID/SETGID/SETPCAP 和绑定端口的 NET_BIND_SERVICE；在启动 NetLite 前清空附加组，降至 UID/GID 10001，并将继承、ambient 和 bounding 能力限定为 NET_BIND_SERVICE。NetLite 与独立 Caddy 均以该非 root 用户运行，保留 no-new-privileges 和只读根文件系统，不修改宿主机 sysctl。管理命令建议使用 `docker compose -f compose.host.yaml exec --user 10001:10001 netlite /opt/netlite/bin/netlite info --data /data`；普通 `exec` 不经过降权入口。

这套配置需要包含 `netlite-host` 引导脚本的新镜像，不能只更新 Compose 后继续使用旧镜像。rootless Docker / 用户命名空间部署的低端口和 host 网络能力可能不同，需单独验收。

## GitHub 构建和镜像发布

工作流：`.github/workflows/container.yml`，镜像仓库由 GitHub 仓库名自动生成并转为小写。本仓库为 `ghcr.io/simlym/netlite`。

| 触发方式 | 行为 / 镜像标签 |
| --- | --- |
| Pull request / codex 分支 | 现有 CI 与 amd64 容器烟测；不登录、不发布 |
| 推送 `main` | 测试后发布 `main` 和 `sha-完整提交哈希` |
| 推送版本标签，例如 `v0.1.5` | 发布 `0.1.5`、`0.1`、`sha-完整提交哈希`；正式版本同时发布 `latest` |
| Actions 手动运行 | 测试后发布提交哈希标签；所选引用满足上述条件时附加对应标签 |

构建顺序：复用现有 CI 的 Linux / Windows、Go 测试 / vet、真实 Caddy 代理烟测、前端测试 / 构建 → Linux amd64 镜像构建及容器烟测 → amd64/arm64 镜像构建发布。容器烟测覆盖非 root、只读根文件系统、管理员初始化与登录、HTTP/HTTPS 内部 CA、原生 DDNS、正常退出、容器重建后的账号/密钥/CA/草稿/已应用配置恢复。测试仅用虚构 DNS 凭据和本地无 IP 接口，不修改公网 DNS。ARM 当前只做交叉编译，仍需 ARM 实机验收。

首次发布与权限核验（已发布过的仓库可按需核对）：

1. 将改动提交并推送到 GitHub。仓库需允许 Actions 运行。`ci.yml` 在既有检查通过后调用容器工作流；也可手动运行 Container（会执行镜像内置测试和容器烟测）。
2. 工作流使用内置 `GITHUB_TOKEN` 和 `packages: write` 推送 GHCR，无需额外配置发布用 PAT。若已有同名包，检查包的 Manage Actions access，确保本仓库有写入权限。
3. 在 Actions 的 **CI** 运行中确认 `container / image` 成功（手动运行时查看 **Container**），再部署对应标签。
4. GHCR 新包默认可能为私有。在 GitHub 的 Packages → netlite → Package settings 中，按实际需求设置可见性。公开包可匿名拉取；私有包需在 Ubuntu 使用具有 `read:packages` 权限的 PAT（classic）执行 `docker login ghcr.io -u 你的GitHub用户名`，在密码提示中输入令牌，不写入 Compose 或提交 Git。

生产部署建议固定经过验收的版本、提交哈希或镜像摘要，避免 `main` / `latest` 随后续发布漂移。已有版本标签不要重复移动。应用升级可能改变数据库，回退镜像前应恢复对应的数据备份。

## Ubuntu bridge 部署（兼容方式）

前置：安装并启动 Docker Engine 和 Compose v2。可参考 [Docker 官方 Ubuntu 安装说明](https://docs.docker.com/engine/install/ubuntu/)。以下命令假设当前用户已获准访问 Docker；否则按本机权限使用 `sudo docker`。

确认要使用的镜像标签已发布且具有拉取权限后，在 Ubuntu 下载部署文件即可，不需要克隆源码或现场构建：

```bash
mkdir -p ~/netlite
cd ~/netlite
curl -fL https://raw.githubusercontent.com/Simlym/NetLite/main/compose.yaml -o compose.yaml
curl -fL https://raw.githubusercontent.com/Simlym/NetLite/main/deploy/docker.env.example -o .env
# 编辑 .env，将 NETLITE_IMAGE 设为 Actions 已发布的版本或 sha 标签。
nano .env
docker compose pull
docker compose up -d --wait
docker compose ps
docker compose logs --tail=100 netlite
```

稳定发布后，可将上述部署文件下载地址的 `main` 换为同一版本 Git 标签，保持镜像与配置一致。

**bridge 模式默认仍仅将管理端口发布到宿主机 `127.0.0.1:18780`**，与 host 模式不同。可以在自己的电脑建立 SSH 隧道：

```bash
ssh -N -L 18780:127.0.0.1:18780 用户名@Ubuntu地址
```

然后打开 `http://127.0.0.1:18780`。首次执行以下命令查看初始化码，在网页创建管理员账号；之后使用账号密码登录，初始化码不能作为业务 API 令牌：

```bash
cd ~/netlite
docker compose exec netlite /opt/netlite/bin/netlite info --data /data
```

需要可信局域网直接管理时，在 `.env` 设置 `NETLITE_MANAGEMENT_BIND=Ubuntu的局域网IPv4地址` 后执行 `docker compose up -d`。若明确设置为 `0.0.0.0`，管理端口会绑定宿主机所有 IPv4 接口，必须考虑初次 HTTP 的明文风险并限制来源；Caddy Admin `20190` 始终不发布。镜像初始使用 HTTP，可在面板设置启用 HTTPS，详见 [Linux 面板说明](LINUX.md)。健康检查兼容同端口 HTTP / HTTPS；HTTPS 探测仅限容器回环地址并跳过证书信任验证。公网管理应通过 HTTPS / SSH / VPN 等受控入口。

## bridge 网络：端口与上游地址

默认发布 `18080/tcp`、`18443/tcp` 和 `18443/udp`，保留已有映射。端口发布不会自动创建代理规则。网页通过部署声明显示外部访问入口，规则仍保存实际容器监听端口。

在 `.env` 设置 `NETLITE_HTTPS_PORT=443` 后执行 `docker compose up -d --wait`，即可将公网 443 映射到容器 18443。网页选择“外部 443”，会显示 `https://你的域名`，无需填写容器端口。HTTP 同理由 `NETLITE_HTTP_PORT` 配置；管理入口不计入业务入口。

自定义端口需要在 `ports` 中增加实际映射，并在 `.env` 的 `NETLITE_EXTRA_PORT_MAPPINGS` 中追加以逗号开头的声明，例如 `,8443:8443`。声明格式是 `外部TCP端口:容器监听端口`；声明本身不会发布端口，必须与实际 Docker 配置一致。页面对未声明的端口显示提示和配置片段。未提供部署声明的自定义容器显示“外部入口未确认”，不会将容器监听成功当作公网可访问。部署信息来自显式环境配置，不读取 Docker Socket。

- 新增其他监听端口，例如 `19080`，需在 `compose.yaml` 的 `ports` 中加入 `"19080:19080/tcp"`，然后执行 `docker compose up -d`。
- 管理界面在容器内监听 `0.0.0.0:18780`，宿主机发布范围由 Compose 控制；Caddy Admin 保持容器内 `127.0.0.1:20190`。
- 上游的 `127.0.0.1` 指向 NetLite 容器自身。宿主机服务可使用 `http://host.docker.internal:端口`；本配置已加入 Linux `host-gateway` 映射。宿主机目标服务必须监听容器可达地址，若仅监听宿主机 `127.0.0.1`，bridge 模式无法直接访问。
- 同一 Docker 网络中的其他容器可用服务名访问；不同 Compose 项目需要显式加入共同网络。不要依赖可能变化的容器 IP。
- 默认映射为同端口。若对外使用 80/443，可将业务服务宿主机 `80:18080/tcp`、`443:18443/tcp`、`443:18443/udp` 映射到容器高位端口；先确认宿主机端口空闲，并配置路由 / 防火墙。不要停止系统 Web 服务来腾端口。
- DNS-01 不需要开放公网验证端口。HTTP-01 由 NetLite 的独立 `18082` 监听器处理，需额外发布 `80:18082/tcp`，并确保公网 TCP 80 可达；此时不能同时把宿主机 80 映射到业务 18080，可将业务 HTTP 入口另设高位端口。默认 Compose 不发布验证端口；当前不支持 TLS-ALPN-01。Caddy 不负责证书签发，需先确认 NetLite 证书状态为 ready，再校验和应用代理。
- 检查实际端口可达范围；Docker 发布端口与宿主机防火墙有独立规则关系，不能仅依据 UFW 状态判断。参考 [Docker 防火墙说明](https://docs.docker.com/engine/network/packet-filtering-firewalls/)。

## 已有 bridge 部署迁移至 host

如果希望在网页新增代理端口后立即监听宿主机，或者需要访问只绑定宿主机回环地址的上游，可使用独立的 `compose.host.yaml`：

```bash
cd ~/netlite
curl -fL https://raw.githubusercontent.com/Simlym/NetLite/main/compose.host.yaml -o compose.host.yaml
# 从 bridge 切换时先停止原容器，保留数据卷。
docker compose down
docker compose -f compose.host.yaml pull
docker compose -f compose.host.yaml up -d --wait
```

两种配置在同一目录、同一 Compose 项目名下复用同一数据卷。不要同时启动，也不要用 `-f compose.yaml -f compose.host.yaml` 合并；后续操作统一带 `-f compose.host.yaml`。

迁移前备份数据并确认新镜像版本。已有规则保留原监听端口，不自动改成 80/443；迁移后可在网页编辑。bridge 中的 `host.docker.internal` 不是 host 模式的内置域名，宿主机目标改用 127.0.0.1；Docker 服务名也不再通过原 bridge DNS 解析，需改为宿主机可访问的地址。迁移不会删除卷或修改其他服务。

host 网络共享宿主机网络空间，`127.0.0.1` 即宿主机；不使用 `ports`。新版本的 `deploy/docker.env.example` 将管理绑定参数留为注释，因此 host 默认网络监听、bridge 默认回环；**旧 `.env` 如仍有 `NETLITE_MANAGEMENT_BIND=127.0.0.1` 会覆盖新默认值**，迁移前需要自行核对。host 管理入口默认监听 `0.0.0.0:18780`，但旧 `.env` 中的显式设置仍会生效。Caddy Admin 需要宿主机 `127.0.0.1:20190` 空闲，占用时应用拒绝启动。HTTP-01 在验证期间监听宿主机 `0.0.0.0:18082`，届时也需空闲；使用公网 HTTP-01 时还需配置公网 TCP 80 到该端口的转发。代理新增端口还需检查宿主机端口冲突和防火墙。本示例面向 Linux，不作为 Windows/macOS 默认方案。

## 数据、备份与升级

`netlite-data` 命名卷挂载到 `/data`，保存数据库（含管理员账号与会话）、初始化码、`proxy-applied.json`、NetLite 证书、日志和 DNS 加密主密钥。Docker 首次创建卷时沿用镜像中 `/data` 的属主 `10001:10001`；容器以该非 root 用户运行。若改用宿主机目录挂载，先创建目录并设为 `10001:10001`、权限 `0700`，不要给目录开放所有用户写权限。

停止服务后备份整个数据目录，避免运行中复制 SQLite 产生不一致。备份可能包含凭据，应作为敏感文件保管。

**下列备份与恢复示例针对推荐的 host 模式。** 若实际使用 bridge 模式，应把每条 `docker compose -f compose.host.yaml` 改为 `docker compose -f compose.yaml`，不要混用两种配置或误操作其他 Compose 项目。


```bash
cd ~/netlite
mkdir -p backups
chmod 700 backups
docker compose -f compose.host.yaml stop
(umask 077; docker compose -f compose.host.yaml run --rm --no-deps -T --entrypoint tar netlite -C /data -czf - . > backups/netlite-data.tgz)
docker compose -f compose.host.yaml up -d --wait
```

确认备份命令成功、归档可读，再进行升级。备份文件名示例固定，实际使用时请另存日期版本。恢复到空数据卷的示例（会向该卷解压文件，务必确认目标卷）：

```bash
docker compose -f compose.host.yaml stop
docker compose -f compose.host.yaml run --rm --no-deps -T --entrypoint tar netlite -C /data -xzf - < backups/netlite-data.tgz
docker compose -f compose.host.yaml up -d --wait
```

升级：备份后修改 `.env` 的镜像标签。host 模式执行 `docker compose -f compose.host.yaml pull && docker compose -f compose.host.yaml up -d --wait`；bridge 模式执行 `docker compose pull && docker compose up -d --wait`。更换容器会保留数据卷；`docker compose down` 默认也保留卷，**不要执行 `docker compose down -v`，它会删除数据卷**。不要丢失 `/data/dns/master.key`，否则即便保存数据库也可能无法解密 DNS 账号。

排查代理运行错误可查看 Caddy 日志：host 模式使用 `docker compose -f compose.host.yaml exec netlite tail -n 100 /data/caddy.log`，bridge 模式使用 `docker compose exec netlite tail -n 100 /data/caddy.log`。**证书是由 NetLite 签发和续期的，Caddy 日志不是证书签发日志。** 申请问题应结合面板中的「HTTPS 证书」状态和错误信息检查。Compose 已限制容器标准输出日志大小；数据卷内的 `caddy.log` 仍需维护。健康检查检测管理 HTTP / HTTPS 服务；`unhealthy` 本身不会触发 Docker 重启。`restart: unless-stopped` 处理进程退出；Caddy 意外退出会使 NetLite 退出。

## 开发阶段推荐流程

日常修改优先在开发机运行 Go / Vite，快速查看反馈；需要测试 Ubuntu 容器行为时，在专用测试机拉取源码，用同一个 Dockerfile 本地构建，不必等每次 GitHub 完整打包。测试与正式环境使用独立数据卷和目录。

```bash
git clone https://github.com/Simlym/NetLite.git
cd NetLite
# 后续更新先 git pull --ff-only
docker compose -f compose.yaml -f compose.build.yaml up -d --build --wait
docker compose -f compose.yaml -f compose.build.yaml logs --tail=100 netlite
```

此模式使用本地 `netlite:dev`，利用 Docker 构建缓存；不推送 GHCR。不变的 npm / Caddy 层可复用，Go 模块和编译使用缓存挂载。修改 Dockerfile 或源码后重跑上述构建命令即可。

`compose.build.yaml` 默认启用本次中国地区 Ubuntu 测试机已验证的下载配置：

| 配置 | 默认值 | 作用 |
| --- | --- | --- |
| `NETLITE_BUILD_REGISTRY` | `docker.m.daocloud.io` | Node、Go、Debian 基础镜像仓库 |
| `NETLITE_BUILDKIT_SYNTAX` | `docker.m.daocloud.io/docker/dockerfile:1` | Dockerfile 构建组件 |
| `NETLITE_GOPROXY` | `https://goproxy.cn` | 后端与 Caddy 的 Go 模块下载，保留依赖校验 |

直接使用上述两份 Compose 文件即可，不需要手工生成 `tools/Dockerfile.mirror`、附加 `tools/compose.mirror.yaml` 或设置环境变量。此前已使用临时文件的测试机，更新代码后改用上面的标准命令，复用同一 Compose 项目数据卷。

如需覆盖默认值，可在 `.env` 或当前终端设置变量。使用官方源的示例：

```bash
export NETLITE_BUILD_REGISTRY=docker.io
export NETLITE_BUILDKIT_SYNTAX=docker/dockerfile:1
export NETLITE_GOPROXY=https://proxy.golang.org,direct
docker compose -f compose.yaml -f compose.build.yaml up -d --build --wait
```

这些配置只影响本地 Compose 构建；直接 `docker build` 和 GitHub 镜像构建默认使用官方镜像仓库与 Go 代理。无需修改 Docker daemon 或重启已有容器。镜像服务可能有白名单或限流；npm 和 Debian apt 下载不使用上述 Go 代理，若这些阶段失败，需按具体错误处理。[DaoCloud 使用说明](https://github.com/DaoCloud/public-image-mirror)、[Goproxy.cn](https://goproxy.cn/)

代码推送后仍由 CI 回归检查。main / 版本标签生成 GHCR 双架构镜像；部署验收和正式部署使用 GitHub 发布的固定版本 / 提交哈希镜像，确保各机器使用同一份产物。无需把每次保存文件都变成一次 GitHub 完整构建。

## 本地构建与验证

```bash
docker build -t netlite:smoke .
python3 scripts/smoke-container.py --image netlite:smoke
NETLITE_IMAGE=netlite:smoke docker compose up -d --wait
```

构建需要访问 npm、Go 模块、GitHub、Docker 镜像源。Caddy 版本与现有脚本保持一致；不构建或携带 ddns-go，也不依赖 Caddy DNS 插件。

部署前应检查对应 GitHub Actions 与 GHCR 产物是否实际发布成功；Ubuntu/VPS、ARM64、fnOS 实机以及真实 DNS / ACME 签发还需分别验收。工作流配置和模拟测试不能替代目标环境的验证结果。

参考：[GitHub 多架构镜像构建](https://docs.docker.com/build/ci/github-actions/multi-platform/)、[GHCR 认证与可见性](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)。
