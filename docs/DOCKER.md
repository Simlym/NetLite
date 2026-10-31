# Docker Compose 部署（Ubuntu / 通用 Linux）

NetLite 镜像包含内嵌前端的 Go 后端和标准 Caddy；DDNS、公共 DNS API 与证书生命周期均由 NetLite 内置管理。由后端管理自己的子进程，不连接宿主机已有 Caddy，也不修改系统 Web 服务。运行时无需 Go、Node.js 或 Python。

支持构建 `linux/amd64` 和 `linux/arm64` 镜像。默认 bridge 网络，另提供 Linux host 网络配置。FPK 构建路径保持独立。

## GitHub 构建和镜像发布

工作流：`.github/workflows/container.yml`，镜像仓库由 GitHub 仓库名自动生成并转为小写。本仓库为 `ghcr.io/simlym/netlite`。

| 触发方式 | 行为 / 镜像标签 |
| --- | --- |
| Pull request / codex 分支 | 现有 CI 与 amd64 容器烟测；不登录、不发布 |
| 推送 `main` | 测试后发布 `main` 和 `sha-完整提交哈希` |
| 推送版本标签，例如 `v0.1.5` | 发布 `0.1.5`、`0.1`、`sha-完整提交哈希`；正式版本同时发布 `latest` |
| Actions 手动运行 | 测试后发布提交哈希标签；所选引用满足上述条件时附加对应标签 |

构建顺序：复用现有 CI 的 Linux / Windows、Go 测试 / vet、真实 Caddy 代理烟测、前端测试 / 构建 → Linux amd64 镜像构建及容器烟测 → amd64/arm64 镜像构建发布。容器烟测覆盖非 root、只读根文件系统、管理员初始化与登录、HTTP/HTTPS 内部 CA、原生 DDNS、正常退出、容器重建后的账号/密钥/CA/草稿/已应用配置恢复。测试仅用虚构 DNS 凭据和本地无 IP 接口，不修改公网 DNS。ARM 当前只做交叉编译，仍需 ARM 实机验收。

首次启用：

1. 将改动提交并推送到 GitHub。仓库需允许 Actions 运行。`ci.yml` 在既有检查通过后调用容器工作流；也可手动运行 Container（会执行镜像内置测试和容器烟测）。
2. 工作流使用内置 `GITHUB_TOKEN` 和 `packages: write` 推送 GHCR，无需额外配置发布用 PAT。若已有同名包，检查包的 Manage Actions access，确保本仓库有写入权限。
3. 在 Actions 的 **CI** 运行中确认 `container / image` 成功（手动运行时查看 **Container**），再部署对应标签。
4. GHCR 新包默认可能为私有。在 GitHub 的 Packages → netlite → Package settings 中，按实际需求设置可见性。公开包可匿名拉取；私有包需在 Ubuntu 使用具有 `read:packages` 权限的 PAT（classic）执行 `docker login ghcr.io -u 你的GitHub用户名`，在密码提示中输入令牌，不写入 Compose 或提交 Git。

生产部署建议固定经过验收的版本、提交哈希或镜像摘要，避免 `main` / `latest` 随后续发布漂移。已有版本标签不要重复移动。应用升级可能改变数据库，回退镜像前应恢复对应的数据备份。

## Ubuntu 首次部署

前置：安装并启动 Docker Engine 和 Compose v2。可参考 [Docker 官方 Ubuntu 安装说明](https://docs.docker.com/engine/install/ubuntu/)。以下命令假设当前用户已获准访问 Docker；否则按本机权限使用 `sudo docker`。

首次镜像发布成功后，在 Ubuntu 下载部署文件即可，不需要克隆源码或现场构建：

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

默认管理端口只发布在 Ubuntu 的 `127.0.0.1:18780`。在自己的电脑建立 SSH 隧道：

```bash
ssh -N -L 18780:127.0.0.1:18780 用户名@Ubuntu地址
```

然后打开 `http://127.0.0.1:18780`。首次执行以下命令查看初始化码，在网页创建管理员账号；之后使用账号密码登录，初始化码不能作为业务 API 令牌：

```bash
cd ~/netlite
docker compose exec netlite /opt/netlite/bin/netlite info --data /data
```

需要可信局域网直接管理时，在 `.env` 设置 `NETLITE_MANAGEMENT_BIND=Ubuntu的局域网IPv4地址` 后执行 `docker compose up -d`。不要将管理端口映射到公网；Caddy Admin `20190` 始终不发布。镜像初始使用 HTTP，可在面板设置启用 HTTPS，详见 [Linux 面板说明](LINUX.md)。健康检查兼容同端口 HTTP / HTTPS；HTTPS 探测仅限容器回环地址并跳过证书信任验证。公网管理应通过 HTTPS / SSH / VPN 等受控入口。

## bridge 网络：端口与上游地址

默认发布 `18080/tcp`、`18443/tcp` 和 `18443/udp`。端口发布不会自动创建代理规则，请在界面添加规则、校验并应用。规则端口仍限定 `1024..65535`。

- 新增其他监听端口，例如 `19080`，需在 `compose.yaml` 的 `ports` 中加入 `"19080:19080/tcp"`，然后执行 `docker compose up -d`。
- 管理界面在容器内监听 `0.0.0.0:18780`，宿主机发布范围由 Compose 控制；Caddy Admin 保持容器内 `127.0.0.1:20190`。
- 上游的 `127.0.0.1` 指向 NetLite 容器自身。宿主机服务可使用 `http://host.docker.internal:端口`；本配置已加入 Linux `host-gateway` 映射。宿主机目标服务必须监听容器可达地址，若仅监听宿主机 `127.0.0.1`，bridge 模式无法直接访问。
- 同一 Docker 网络中的其他容器可用服务名访问；不同 Compose 项目需要显式加入共同网络。不要依赖可能变化的容器 IP。
- 默认映射为同端口。若对外使用 80/443，可将业务服务宿主机 `80:18080/tcp`、`443:18443/tcp`、`443:18443/udp` 映射到容器高位端口；先确认宿主机端口空闲，并配置路由 / 防火墙。不要停止系统 Web 服务来腾端口。
- DNS-01 不需要开放公网验证端口。HTTP-01 由 NetLite 的独立 `18082` 监听器处理，需额外发布 `80:18082/tcp`，并确保公网 TCP 80 可达；此时不能同时把宿主机 80 映射到业务 18080，可将业务 HTTP 入口另设高位端口。默认 Compose 不发布验证端口；当前不支持 TLS-ALPN-01。Caddy 不负责证书签发，需先确认 NetLite 证书状态为 ready，再校验和应用代理。
- 检查实际端口可达范围；Docker 发布端口与宿主机防火墙有独立规则关系，不能仅依据 UFW 状态判断。参考 [Docker 防火墙说明](https://docs.docker.com/engine/network/packet-filtering-firewalls/)。

## Ubuntu host 网络（可选）

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

host 网络共享宿主机网络空间，`127.0.0.1` 即宿主机；不使用 `ports`。默认管理监听仍为 `127.0.0.1:18780`。Caddy Admin 需要宿主机 `127.0.0.1:20190` 空闲，占用时应用拒绝启动。HTTP-01 在验证期间监听宿主机 `0.0.0.0:18082`，届时也需空闲；使用公网 HTTP-01 时还需配置公网 TCP 80 到该端口的转发。代理新增端口还需检查宿主机端口冲突和防火墙。本示例面向 Linux，不作为 Windows/macOS 默认方案。

## 数据、备份与升级

`netlite-data` 命名卷挂载到 `/data`，保存数据库（含管理员账号与会话）、初始化码、`proxy-applied.json`、NetLite 证书、日志和 DNS 加密主密钥。Docker 首次创建卷时沿用镜像中 `/data` 的属主 `10001:10001`；容器以该非 root 用户运行。若改用宿主机目录挂载，先创建目录并设为 `10001:10001`、权限 `0700`，不要给目录开放所有用户写权限。

停止服务后备份整个数据目录，避免运行中复制 SQLite 产生不一致。备份可能包含凭据，应作为敏感文件保管：

```bash
cd ~/netlite
mkdir -p backups
chmod 700 backups
docker compose stop
(umask 077; docker compose run --rm --no-deps -T --entrypoint tar netlite -C /data -czf - . > backups/netlite-data.tgz)
docker compose up -d --wait
```

确认备份命令成功、归档可读，再进行升级。备份文件名示例固定，实际使用时请另存日期版本。恢复到空数据卷的示例（会向该卷解压文件，务必确认目标卷）：

```bash
docker compose stop
docker compose run --rm --no-deps -T --entrypoint tar netlite -C /data -xzf - < backups/netlite-data.tgz
docker compose up -d --wait
```

升级：备份后修改 `.env` 的镜像标签，再执行 `docker compose pull && docker compose up -d --wait`。更换容器保留卷；`docker compose down` 默认保留卷，**不要执行 `docker compose down -v`，它会删除数据卷**。不要丢失 `/data/dns/master.key`，只有数据库无法解密 DNS 账号。

查看 Caddy 签发日志：`docker compose exec netlite tail -n 100 /data/caddy.log`。Compose 限制了容器标准输出日志大小，`caddy.log` 是数据卷内文件，仍需定期维护。健康检查检测管理 HTTP / HTTPS 服务；`unhealthy` 本身不会触发 Docker 重启。`restart: unless-stopped` 处理进程退出；Caddy 意外退出会使 NetLite 退出。

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

如果测试机无法直连 Docker Hub，但已实测可访问 DaoCloud，可在本地构建前设置以下变量（也可写入被 Git 忽略的 `.env`），无需修改 Docker daemon 或重启已有容器：

```bash
export NETLITE_BUILD_REGISTRY=docker.m.daocloud.io
export NETLITE_BUILDKIT_SYNTAX=docker.m.daocloud.io/docker/dockerfile:1
docker compose -f compose.yaml -f compose.build.yaml up -d --build --wait
```

第一个变量替换 Node、Go、Debian 基础镜像仓库，第二个替换 Dockerfile 构建组件的来源。镜像服务可能有白名单或限流；某个镜像拉取成功并不保证其他镜像与标签都可用。npm、Go 模块和 Debian apt 的下载独立于镜像仓库，若这些阶段失败，需按具体错误另行处理。[DaoCloud 使用说明](https://github.com/DaoCloud/public-image-mirror)

代码推送后仍由 CI 回归检查。main / 版本标签生成 GHCR 双架构镜像；部署验收和正式部署使用 GitHub 发布的固定版本 / 提交哈希镜像，确保各机器使用同一份产物。无需把每次保存文件都变成一次 GitHub 完整构建。

## 本地构建与验证

```bash
docker build -t netlite:smoke .
python3 scripts/smoke-container.py --image netlite:smoke
NETLITE_IMAGE=netlite:smoke docker compose up -d --wait
```

构建需要访问 npm、Go 模块、GitHub、Docker 镜像源。Caddy 版本与现有脚本保持一致；不构建或携带 ddns-go，也不依赖 Caddy DNS 插件。

首次 GitHub Actions / GHCR 发布、Ubuntu 安装、ARM 实机以及真实 DNS / ACME 签发需分别验收。仓库中提供构建和自动检查流程，并不代表这些外部环境已完成验证。

参考：[GitHub 多架构镜像构建](https://docs.docker.com/build/ci/github-actions/multi-platform/)、[GHCR 认证与可见性](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)。
