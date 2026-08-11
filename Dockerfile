FROM alpine AS base

LABEL maintainer="meteormin"

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

RUN go build -trimpath -ldflags="-X 'main.version=${VERSION}' -X 'main.buildTime=${BUILD_TIME}'" -o build/server ./cmd/server/main.go

FROM base AS deploy

ENV SERVER_PORT=3000

WORKDIR /app

RUN apk add curl

COPY --from=go /govfs/build/server ./server
COPY --from=go /govfs/config.toml ./config.toml

RUN mkdir -p /app/data

EXPOSE ${SERVER_PORT}

ENTRYPOINT ["./server"]
