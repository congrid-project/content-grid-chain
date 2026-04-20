FROM docker.io/library/golang:1.25.1-bookworm AS go-builder-base

WORKDIR /src

COPY go.mod go.sum ./
COPY third_party/clientv2/go.mod ./third_party/clientv2/go.mod

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0

FROM go-builder-base AS node-builder

RUN mkdir -p /out /tmp/gomodcache /tmp/gobuildcache \
 && GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gobuildcache GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/content-grid-d ./cmd/content-grid-d \
 && rm -rf /tmp/gomodcache /tmp/gobuildcache

FROM go-builder-base AS operator-builder

RUN mkdir -p /out /tmp/gomodcache /tmp/gobuildcache \
 && GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gobuildcache GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/content-grid-d ./cmd/content-grid-d \
 && GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gobuildcache GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/verifierd ./offchain/verifierd \
 && GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gobuildcache GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/indexerd ./offchain/indexerd \
 && GOMODCACHE=/tmp/gomodcache GOCACHE=/tmp/gobuildcache GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/drand-relayer ./offchain/drandrelayer \
 && rm -rf /tmp/gomodcache /tmp/gobuildcache

FROM docker.io/library/debian:bookworm-slim AS node-runtime

ENV CONGRID_HOME=/var/lib/congrid \
    CONTENT_GRID_BIN=/usr/local/bin/content-grid-d

WORKDIR /opt/congrid

RUN apt-get update \
 && apt-get install -y --no-install-recommends bash ca-certificates curl jq tini \
 && rm -rf /var/lib/apt/lists/*

COPY docker/common.sh /usr/local/lib/congrid/common.sh
COPY docker/node-entrypoint.sh /usr/local/bin/congrid-node-entrypoint
COPY docker/validator-cli.sh /usr/local/bin/congrid-validator-cli
COPY --from=node-builder /out/content-grid-d /usr/local/bin/content-grid-d

RUN chmod 0755 \
  /usr/local/lib/congrid/common.sh \
  /usr/local/bin/congrid-node-entrypoint \
  /usr/local/bin/congrid-validator-cli \
  /usr/local/bin/content-grid-d

VOLUME ["/var/lib/congrid"]

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["content-grid-d", "version"]

FROM docker.io/library/python:3.11-slim-bookworm AS operator-runtime

ENV PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    CONGRID_HOME=/var/lib/congrid \
    CONTENT_GRID_BIN=/usr/local/bin/content-grid-d

WORKDIR /opt/congrid

RUN apt-get update \
 && apt-get install -y --no-install-recommends bash ca-certificates curl jq tini \
 && rm -rf /var/lib/apt/lists/*

COPY offchain/chromad/requirements.txt /opt/congrid/offchain/chromad/requirements.txt
RUN pip install --no-cache-dir -r /opt/congrid/offchain/chromad/requirements.txt

COPY offchain/chromad/server.py /opt/congrid/offchain/chromad/server.py
COPY docker/common.sh /usr/local/lib/congrid/common.sh
COPY docker/node-entrypoint.sh /usr/local/bin/congrid-node-entrypoint
COPY docker/validator-cli.sh /usr/local/bin/congrid-validator-cli
COPY docker/indexerd-entrypoint.sh /usr/local/bin/congrid-indexerd-entrypoint
COPY docker/verifierd-entrypoint.sh /usr/local/bin/congrid-verifierd-entrypoint
COPY docker/drand-relayer-entrypoint.sh /usr/local/bin/congrid-drand-relayer-entrypoint
COPY docker/chromad-entrypoint.sh /usr/local/bin/congrid-chromad-entrypoint
COPY --from=operator-builder /out/content-grid-d /usr/local/bin/content-grid-d
COPY --from=operator-builder /out/verifierd /usr/local/bin/verifierd
COPY --from=operator-builder /out/indexerd /usr/local/bin/indexerd
COPY --from=operator-builder /out/drand-relayer /usr/local/bin/drand-relayer

RUN chmod 0755 \
  /usr/local/lib/congrid/common.sh \
  /usr/local/bin/congrid-node-entrypoint \
  /usr/local/bin/congrid-validator-cli \
  /usr/local/bin/congrid-indexerd-entrypoint \
  /usr/local/bin/congrid-verifierd-entrypoint \
  /usr/local/bin/congrid-drand-relayer-entrypoint \
  /usr/local/bin/congrid-chromad-entrypoint \
  /usr/local/bin/content-grid-d \
  /usr/local/bin/verifierd \
  /usr/local/bin/indexerd \
  /usr/local/bin/drand-relayer

VOLUME ["/var/lib/congrid", "/var/lib/congrid/chroma"]

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["content-grid-d", "version"]
