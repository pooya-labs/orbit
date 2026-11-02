# Build the security-fixed upstream Tempo v3.1.1 release with patched dependencies.
FROM golang:1.27-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
ARG TEMPO_VERSION=v3.1.1
ARG TEMPO_COMMIT=9db7df79d2af105a90d96484e6403e1fda0d011e
RUN git init . \
    && git remote add origin https://github.com/grafana/tempo.git \
    && git fetch --depth=1 origin "refs/tags/${TEMPO_VERSION}:refs/tags/${TEMPO_VERSION}" \
    && test "$(git rev-parse "refs/tags/${TEMPO_VERSION}^{commit}")" = "${TEMPO_COMMIT}" \
    && git checkout --detach "refs/tags/${TEMPO_VERSION}" \
    && test "$(git describe --tags --exact-match)" = "${TEMPO_VERSION}"
RUN go mod edit -require=github.com/go-jose/go-jose/v4@v4.1.7 \
    && go mod tidy \
    && go mod verify
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -trimpath \
    -ldflags="-s -w -X main.Version=3.1.1" \
    -o /out/tempo ./cmd/tempo \
    && go version -m /out/tempo | tee /tmp/tempo-build-info \
    && grep -F "vcs.revision=${TEMPO_COMMIT}" /tmp/tempo-build-info

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /var/tempo \
    && chown 10001:10001 /var/tempo
COPY --from=build /out/tempo /tempo
USER 10001:10001
EXPOSE 3200 4317
ENTRYPOINT ["/tempo"]
