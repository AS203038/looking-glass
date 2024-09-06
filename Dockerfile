FROM bufbuild/buf:latest AS buf-builder
WORKDIR /opt/protobuf
COPY protobuf /opt/protobuf
RUN buf generate

FROM node:alpine AS node-builder
WORKDIR /opt
COPY webui /opt/webui
COPY --from=buf-builder /opt/protobuf /opt/protobuf
RUN cd protobuf \
 && npm install \
 && cd ../webui \
 && npm install \
 && npm run build

FROM golang:alpine AS go-builder
ENV CGO_ENABLED=0
ARG VERSION=untracked
WORKDIR /opt
COPY pkg /opt/pkg
COPY cmd/server /opt/cmd/server
COPY go.mod go.sum /opt/.
COPY --from=buf-builder /opt/protobuf /opt/protobuf
COPY --from=node-builder /opt/cmd/server/dist /opt/cmd/server/dist
RUN go mod tidy \
 && go build -ldflags="-X github.com/AS203038/looking-glass/pkg/utils.release=${VERSION}" -o /opt/looking-glass /opt/cmd/server

FROM scratch
LABEL org.opencontainers.image.source https://github.com/AS203038/looking-glass
LABEL org.opencontainers.image.description Yet another looking glass project
LABEL org.opencontainers.image.licenses GPL-3.0-or-later
WORKDIR /
COPY --from=go-builder /opt/looking-glass /looking-glass
ENTRYPOINT ["/looking-glass"]
