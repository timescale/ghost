ARG GO_VERSION=1.26
ARG BINARY_SOURCE=builder

# When performing a multi-platform build, leverage Go's built-in support for
# cross-compilation instead of relying on emulation (which is much slower).
# See: https://docs.docker.com/build/building/multi-platform/#cross-compiling-a-go-application
FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS builder
ARG TARGETOS
ARG TARGETARCH

# Download dependencies to local module cache
WORKDIR /src
RUN --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download -x

# Build static executable
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -o /bin/ghost ./cmd/ghost

# When building Docker images via GoReleaser, the binaries are built externally
# and copied in. See: https://goreleaser.com/customization/dockers_v2/
FROM scratch AS release
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/ghost /bin/ghost

# Either the 'builder' or 'release' stage, depending on whether we're building
# the binaries in Docker or outside via GoReleaser.
FROM ${BINARY_SOURCE} AS binary_source

# Create final alpine image
FROM alpine:3.23 AS final

# Install psql for sake of `ghost psql`
RUN apk add --update --no-cache postgresql-client

# Create non-root user/group
RUN addgroup -g 1000 ghost && adduser -u 1000 -G ghost -D ghost
USER ghost
WORKDIR /home/ghost

# Set env vars to control default Ghost behavior
ENV GHOST_CONFIG_DIR=/home/ghost/.config/ghost

# Pre-create config directory so it has the correct ownership (ghost:ghost)
# when Docker initializes the anonymous volume.
RUN mkdir -p /home/ghost/.config/ghost

# Declare config file mount points
VOLUME /home/ghost/.config/ghost
VOLUME /home/ghost/.pgpass

# Copy binary to final image
COPY --from=binary_source /bin/ghost /usr/local/bin/ghost

ENTRYPOINT ["ghost"]
CMD ["mcp", "start"]
