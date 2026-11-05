# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p ../internal/web/dist && npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/apisix-dashboard ./cmd/apisix-dashboard \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
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
