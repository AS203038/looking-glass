# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM bufbuild/buf:latest AS buf-builder
WORKDIR /opt/protobuf
COPY protobuf/buf.yaml protobuf/buf.gen.yaml ./
COPY protobuf/lookingglass ./lookingglass
RUN buf generate

FROM --platform=$BUILDPLATFORM node:alpine AS node-builder
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN npm install -g pnpm
WORKDIR /opt
COPY webui/pnpm-workspace.yaml webui/package.json webui/pnpm-lock.yaml ./webui/
COPY protobuf/package.json ./protobuf/
RUN --mount=type=cache,target=/pnpm/store \
    cd webui \
 && pnpm config set store-dir /pnpm/store \
 && pnpm install --frozen-lockfile
COPY webui ./webui
COPY --from=buf-builder /opt/protobuf ./protobuf
RUN cd webui && pnpm run build

FROM --platform=$BUILDPLATFORM golang:alpine AS go-builder
ENV CGO_ENABLED=0
ENV GOFLAGS=-buildvcs=false
ARG VERSION=untracked
ARG TARGETOS TARGETARCH
WORKDIR /opt
RUN apk --no-cache add ca-certificates
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY pkg ./pkg
COPY cmd/server ./cmd/server
COPY --from=buf-builder /opt/protobuf ./protobuf
COPY --from=node-builder /opt/cmd/server/dist ./cmd/server/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags="-s -w -X github.com/AS203038/looking-glass/pkg/utils.release=${VERSION}" \
      -o /opt/looking-glass ./cmd/server

FROM scratch AS final
ARG VERSION=untracked
LABEL org.opencontainers.image.source=https://github.com/AS203038/looking-glass
LABEL org.opencontainers.image.description="Yet another looking glass project"
LABEL org.opencontainers.image.licenses=GPL-3.0-or-later
LABEL org.opencontainers.image.version=$VERSION
WORKDIR /
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go-builder /opt/looking-glass /looking-glass
USER 65532:65532
ENTRYPOINT ["/looking-glass"]
