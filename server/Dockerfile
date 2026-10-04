# Build context: cozycast-next/ (the server embeds the web build).

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p ../server/webui/dist && npm run build

FROM golang:1.26-alpine AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/server/webui/dist ./webui/dist
# The build cache is kept between builds (a BuildKit cache mount): without it
# every build compiles all dependencies again, the SQLite driver alone for
# minutes on a small server.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cozycast .
# The image has no shell; prepare the data dir here so a fresh volume is
# owned by the nonroot user.
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /cozycast /cozycast
COPY --from=server --chown=65532:65532 /data /data
EXPOSE 8080 8443
ENTRYPOINT ["/cozycast"]
