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
COPY server /opt/server
COPY go.mod go.sum main.go /opt/.
COPY --from=buf-builder /opt/protobuf /opt/protobuf
COPY --from=node-builder /opt/dist /opt/dist
RUN go mod tidy \
 && go build -ldflags="-X github.com/AS203038/looking-glass/server/utils.release=${VERSION}" -o /opt/looking-glass

FROM scratch
WORKDIR /
COPY --from=go-builder /opt/looking-glass /looking-glass

ENTRYPOINT ["/looking-glass"]
