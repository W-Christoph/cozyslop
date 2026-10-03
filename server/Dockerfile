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
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cozycast .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /cozycast /cozycast
EXPOSE 8080
ENTRYPOINT ["/cozycast"]
