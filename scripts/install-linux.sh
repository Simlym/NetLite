#!/usr/bin/env bash
# Run from an extracted Linux release. No runtime Go, Node.js or Python needed.
set -euo pipefail
[[ ${EUID} == 0 ]] || { echo '请使用 sudo bash install.sh' >&2; exit 1; }
command -v systemctl >/dev/null || { echo '需要使用 systemd 的 Linux 系统' >&2; exit 1; }
bundle=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
[[ -x "$bundle/bin/netlite" && -x "$bundle/bin/caddy" ]] || { echo '请从完整 Linux 发布包运行安装脚本' >&2; exit 1; }
[[ ! -e /opt/netlite/bin/netlite ]] || { echo '检测到已安装的 NetLite；请按 docs/LINUX.md 升级，不覆盖访问设置。' >&2; exit 1; }
umask 077
read -r -p '管理端口 [18780]：' port
port=${port:-18780}
[[ "$port" =~ ^[0-9]{4,5}$ && "$port" -ge 1024 && "$port" -le 65535 && "$port" != 20190 && "$port" != 18082 ]] || { echo '端口须为 1024–65535，不能使用 20190 或 18082' >&2; exit 1; }
read -r -p '访问方式：1 局域网/网络访问，2 仅本机 [1]：' access
case ${access:-1} in 1) listen="0.0.0.0:$port" ;; 2) listen="127.0.0.1:$port" ;; *) echo '请选择 1 或 2' >&2; exit 1 ;; esac
if command -v ss >/dev/null && ss -H -ltn "sport = :$port" | read -r _; then echo '管理端口已占用，请更换端口' >&2; exit 1; fi
read -r -p '管理员用户名 [admin]：' username
username=${username:-admin}
read -r -s -p '管理员密码（至少 12 字节；回车生成临时密码）：' password
printf '\n'
temporary=false
if [[ -z "$password" ]]; then
 password=$(od -An -N 20 -tx1 /dev/urandom | tr -d ' \n'); temporary=true
else
 read -r -s -p '再次输入密码：' repeated
 printf '\n'
 [[ "$password" == "$repeated" ]] || { echo '两次密码不一致' >&2; exit 1; }
fi
password_bytes=$(printf '%s' "$password" | wc -c)
[[ "$password_bytes" -ge 12 && "$password_bytes" -le 72 ]] || { echo '密码长度须为 12–72 字节' >&2; exit 1; }
getent group netlite >/dev/null || groupadd --system netlite
id netlite >/dev/null 2>&1 || useradd --system --gid netlite --home-dir /var/lib/netlite --shell /usr/sbin/nologin netlite
install -d -m 755 /opt/netlite /opt/netlite/bin /etc/netlite
install -d -o netlite -g netlite -m 700 /var/lib/netlite
args=(init-admin --data /var/lib/netlite --username "$username")
[[ "$temporary" == false ]] || args+=(--temporary)
printf '%s\n' "$password" | "$bundle/bin/netlite" "${args[@]}"
for database_file in /var/lib/netlite/netlite.db /var/lib/netlite/netlite.db-wal /var/lib/netlite/netlite.db-shm; do
 [[ ! -e "$database_file" ]] || chown netlite:netlite "$database_file"
done
install -m 755 "$bundle/bin/netlite" /opt/netlite/bin/netlite
install -m 755 "$bundle/bin/caddy" /opt/netlite/bin/caddy
printf 'NETLITE_LISTEN=%s\n' "$listen" > /etc/netlite/netlite.env
chmod 644 /etc/netlite/netlite.env
install -m 644 "$bundle/deploy/netlite.service" /etc/systemd/system/netlite.service
ln -sf /opt/netlite/bin/netlite /usr/local/bin/netlite
systemctl daemon-reload
systemctl enable --now netlite
for ((i=0;i<20;i++)); do
 if (exec 3<>/dev/tcp/127.0.0.1/"$port"; printf 'GET /healthz HTTP/1.0\r\nHost: localhost\r\n\r\n' >&3; read -r -t 2 response <&3; [[ "$response" == *' 200 '* ]]) 2>/dev/null; then ready=true; break; fi
 sleep 1
done
[[ ${ready:-false} == true ]] || { echo '服务未就绪。请执行 journalctl -u netlite 查看日志。' >&2; exit 1; }
printf '\nNetLite 已启动\n'
if [[ "$access" == 2 ]]; then
 printf '本机访问：http://127.0.0.1:%s\n' "$port"
else
 for address in $(hostname -I); do
  [[ "$address" != *:* ]] || address="[$address]"
  printf '访问地址：http://%s:%s\n' "$address" "$port"
 done
 echo '外部可达性尚未验证；云服务器需开放安全组，系统防火墙需允许管理端口。公网使用请启用 HTTPS。'
fi
printf '管理员：%s\n' "$username"
[[ "$temporary" == false ]] || printf '临时密码：%s\n首次登录后请修改密码。\n' "$password"
unset password repeated
printf '查看信息：sudo netlite info\n重置密码：sudo netlite reset-password（从标准输入读取）\n'
