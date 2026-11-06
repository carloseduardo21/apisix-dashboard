# syntax=docker/dockerfile:1
# Base images are pinned by digest; Dependabot keeps them up to date.

# Build stages run on the build machine's platform and cross-compile, so
# multi-arch images (linux/amd64, linux/arm64) need no emulation.
FROM --platform=$BUILDPLATFORM node:25-alpine@sha256:bdf2cca6fe3dabd014ea60163eca3f0f7015fbd5c7ee1b0e9ccb4ced6eb02ef4 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p ../internal/web/dist && npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/apisix-dashboard ./cmd/apisix-dashboard \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
LABEL org.opencontainers.image.title="Lintel for Apache APISIX" \
      org.opencontainers.image.description="Secure self-hosted dashboard for the Apache APISIX Admin API (not an Apache project)" \
      org.opencontainers.image.source="https://github.com/carloseduardo21/apisix-dashboard" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY --from=build /out/apisix-dashboard /apisix-dashboard
COPY --from=build --chown=65532:65532 /out/data /data
ENV DASH_DATA_DIR=/data \
    DASH_LISTEN_ADDR=:8080
VOLUME /data
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/apisix-dashboard", "healthcheck"]
ENTRYPOINT ["/apisix-dashboard"]
CMD ["serve"]
