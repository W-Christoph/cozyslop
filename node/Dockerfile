# The agent on a computer that runs a room for a CozyCast server elsewhere
# (docs/home-hosting.md, compose.node.yaml). Build context: cozycast-next/.
# Builds for any platform (e.g. --platform linux/arm64) without emulation:
# the build stage runs on the building machine and Go cross-compiles.

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /cozycast-node ./cmd/node
# The image has no shell; prepare the volumes' folders here so fresh volumes
# are owned by the nonroot user.
RUN mkdir -p /state /run/cozycast

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /cozycast-node /cozycast-node
COPY --from=build --chown=65532:65532 /state /state
COPY --from=build --chown=65532:65532 /run/cozycast /run/cozycast
ENTRYPOINT ["/cozycast-node"]
