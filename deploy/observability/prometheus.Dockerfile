# Build the pinned Prometheus v3.16.0 source with the fixed golang.org/x/net release.
FROM golang:1.27-alpine AS build
RUN apk add --no-cache bash git make nodejs npm
WORKDIR /src
ARG TARGETARCH
ARG PROMETHEUS_VERSION=v3.16.0
ARG PROMETHEUS_COMMIT=e2e28adeee2c5883d000fc82aca6008cfbf7e96d
RUN git init . \
    && git remote add origin https://github.com/prometheus/prometheus.git \
    && git fetch --depth=1 origin "refs/tags/${PROMETHEUS_VERSION}:refs/tags/${PROMETHEUS_VERSION}" \
    && test "$(git rev-parse "refs/tags/${PROMETHEUS_VERSION}^{commit}")" = "${PROMETHEUS_COMMIT}" \
    && git checkout --detach "refs/tags/${PROMETHEUS_VERSION}"
RUN node --version \
    && npm install --global pnpm@10.9.0
RUN go mod edit -require=golang.org/x/net@v0.62.0 \
    && go mod tidy \
    && go mod verify \
    && make build \
    && go version -m .build/linux-${TARGETARCH}/prometheus | tee /tmp/prometheus-build-info \
    && grep -F "golang.org/x/net v0.62.0" /tmp/prometheus-build-info

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /prometheus /etc/prometheus \
    && addgroup -g 10001 -S prometheus \
    && adduser -u 10001 -S prometheus -G prometheus \
    && chown -R 10001:10001 /prometheus /etc/prometheus
ARG TARGETARCH
COPY --from=build /src/.build/linux-${TARGETARCH}/prometheus /bin/prometheus
COPY --from=build /src/.build/linux-${TARGETARCH}/promtool /bin/promtool
COPY --from=build /src/LICENSE /LICENSE
COPY --from=build /src/NOTICE /NOTICE
USER 10001:10001
WORKDIR /prometheus
EXPOSE 9090
VOLUME ["/prometheus"]
ENTRYPOINT ["/bin/prometheus"]
CMD ["--config.file=/etc/prometheus/prometheus.yml", "--storage.tsdb.path=/prometheus"]
