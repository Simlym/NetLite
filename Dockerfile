# syntax=docker/dockerfile:1
ARG GO_VERSION=1.27.0
ARG NODE_VERSION=22
ARG BASE_REGISTRY=docker.io

FROM --platform=$BUILDPLATFORM ${BASE_REGISTRY}/library/node:${NODE_VERSION}-bookworm-slim AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
RUN npm test && npm run build

FROM --platform=$BUILDPLATFORM ${BASE_REGISTRY}/library/golang:${GO_VERSION}-bookworm AS go-base
ENV CGO_ENABLED=0 GOFLAGS=-p=2
RUN mkdir -p /out

FROM go-base AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/*.go web/index.html ./web/
COPY --from=web /src/web/dist/ ./web/dist/
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go test -tags webembed ./... && go vet -tags webembed ./... \
    && GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags webembed -trimpath \
    -ldflags='-s -w' -o /out/netlite ./cmd/netlite

FROM go-base AS caddy
WORKDIR /src/caddy
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod init netlite-caddy-build \
    && go get github.com/caddyserver/caddy/v2/cmd/caddy@v2.11.7
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' \
    -o /out/caddy github.com/caddyserver/caddy/v2/cmd/caddy \
    && cp /go/pkg/mod/github.com/caddyserver/caddy/v2@v2.11.7/LICENSE /out/caddy-LICENSE

FROM ${BASE_REGISTRY}/library/debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 netlite \
    && useradd --uid 10001 --gid netlite --no-create-home --home-dir /data --shell /usr/sbin/nologin netlite \
    && install -d -o netlite -g netlite -m 0700 /data
COPY --from=backend --chmod=0755 /out/netlite /opt/netlite/bin/netlite
COPY --from=caddy --chmod=0755 /out/caddy /opt/netlite/bin/caddy
COPY --from=caddy /out/caddy-LICENSE /opt/netlite/licenses/caddy-LICENSE
COPY THIRD_PARTY_NOTICES.md docs/licenses/acmez-LICENSE.txt /opt/netlite/licenses/
ENV HOME=/data XDG_DATA_HOME=/data/xdg/data XDG_CONFIG_HOME=/data/xdg/config
WORKDIR /opt/netlite
USER 10001:10001
EXPOSE 18780/tcp 18080/tcp 18443/tcp 18443/udp
# The panel can switch to HTTPS on the same port. Never follow its public-domain redirect.
HEALTHCHECK --interval=30s --timeout=10s --start-period=20s --retries=3 \
    CMD curl -fsS --max-time 3 http://127.0.0.1:18780/healthz | grep -q '"status":"ok"' \
    || curl -fkSs --max-time 3 https://127.0.0.1:18780/healthz | grep -q '"status":"ok"'
ENTRYPOINT ["/opt/netlite/bin/netlite"]
CMD ["--listen", "0.0.0.0:18780", "--data", "/data", "--caddy", "/opt/netlite/bin/caddy"]
