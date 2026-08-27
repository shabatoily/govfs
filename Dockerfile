FROM alpine AS base

LABEL maintainer="meteormin"

RUN apk add curl

FROM node:24.15-alpine AS node

WORKDIR /webui

COPY webui/package.json webui/yarn.lock ./

RUN yarn install --frozen-lockfile

COPY webui .

RUN yarn build

FROM golang:1.26-alpine AS go

ARG VERSION=dev
ARG BUILD_TIME="unknown"

WORKDIR /govfs

COPY go.mod go.sum ./

RUN go mod download

COPY . .

COPY --from=node /webui/dist ./webui/dist

RUN go build -trimpath -ldflags="-X 'main.version=${VERSION}' -X 'main.buildTime=${BUILD_TIME}'" -o govfs ./cmd/govfs/main.go

FROM base AS govfs

ENV SERVER_PORT=3000

RUN addgroup -g 1000 govfs && \
    adduser -u 1000 -G govfs -s /bin/sh -D govfs && \
    mkdir -p /home/govfs/.govfs /etc/govfs && \
    chown govfs:govfs /home/govfs/.govfs

WORKDIR /home/govfs

USER govfs

COPY --from=go /govfs/govfs /usr/local/bin/govfs
COPY --from=go /govfs/config.toml /etc/govfs/config.toml

EXPOSE ${SERVER_PORT}

ENTRYPOINT ["govfs", "--config", "/etc/govfs/config.toml"]
