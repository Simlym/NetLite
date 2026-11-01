#!/bin/sh
# Host networking needs low-port binding even when the host's privileged-port
# threshold is 1024. Drop bootstrap privileges before starting either service.
set -eu
exec setpriv --reuid=10001 --regid=10001 --clear-groups \
  --inh-caps=-all,+net_bind_service --ambient-caps=-all,+net_bind_service \
  --bounding-set=-all,+net_bind_service --no-new-privs \
  /opt/netlite/bin/netlite "$@"
