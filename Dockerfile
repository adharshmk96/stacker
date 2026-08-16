# syntax=docker/dockerfile:1.7

FROM oven/bun:1-alpine AS ui
WORKDIR /src/stacker-ui
COPY stacker-ui/package.json stacker-ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY stacker-ui/ ./
RUN bun run generate

FROM golang:1.26-alpine AS server
ARG STACKER_VERSION=development
ARG STACKER_BUILT_AT
WORKDIR /src/stacker-server
COPY stacker-server/go.mod stacker-server/go.sum ./
RUN go mod download
COPY stacker-server/ ./
COPY --from=ui /src/stacker-ui/.output/public ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
  -ldflags="-s -w -X stacker/internal/modules/serversettings.Version=${STACKER_VERSION} -X stacker/internal/modules/serversettings.BuiltAt=${STACKER_BUILT_AT}" \
  -o /out/stacker .

FROM alpine:3.23
RUN apk add --no-cache ca-certificates docker-cli openssh-client sshpass tzdata
COPY --from=server /out/stacker /usr/local/bin/stacker
RUN mkdir -p /data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1
ENTRYPOINT ["stacker"]
