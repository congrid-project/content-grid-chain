FROM golang:1.25.1-bookworm AS go-builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/content-grid-d ./cmd/content-grid-d \
 && GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/verifierd ./offchain/verifierd \
 && GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/indexerd ./offchain/indexerd \
 && GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/drand-relayer ./offchain/drandrelayer

FROM python:3.11-slim-bookworm

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
COPY docker/indexerd-entrypoint.sh /usr/local/bin/congrid-indexerd-entrypoint
COPY docker/verifierd-entrypoint.sh /usr/local/bin/congrid-verifierd-entrypoint
COPY docker/drand-relayer-entrypoint.sh /usr/local/bin/congrid-drand-relayer-entrypoint
COPY docker/chromad-entrypoint.sh /usr/local/bin/congrid-chromad-entrypoint
COPY --from=go-builder /out/content-grid-d /usr/local/bin/content-grid-d
COPY --from=go-builder /out/verifierd /usr/local/bin/verifierd
COPY --from=go-builder /out/indexerd /usr/local/bin/indexerd
COPY --from=go-builder /out/drand-relayer /usr/local/bin/drand-relayer

RUN chmod 0755 \
  /usr/local/lib/congrid/common.sh \
  /usr/local/bin/congrid-node-entrypoint \
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
