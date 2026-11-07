# Build the pinned Loki v3.7.9 source with the fixed golang.org/x/net release.
FROM golang:1.27-alpine AS build
RUN apk add --no-cache git make
WORKDIR /src/loki
ARG LOKI_VERSION=v3.7.9
ARG LOKI_COMMIT=d3b4a2f76bfc5625abee7ceb00b8896fa3c31549
RUN git init . \
    && git remote add origin https://github.com/grafana/loki.git \
    && git fetch --depth=1 origin "refs/tags/${LOKI_VERSION}:refs/tags/${LOKI_VERSION}" \
    && test "$(git rev-parse "refs/tags/${LOKI_VERSION}^{commit}")" = "${LOKI_COMMIT}" \
    && git checkout --detach "refs/tags/${LOKI_VERSION}" \
    && test "$(git describe --tags --exact-match)" = "${LOKI_VERSION}"
RUN go mod edit -require=golang.org/x/net@v0.62.0 \
    && go mod tidy \
    && go mod verify \
    && make clean \
    && make BUILD_IN_CONTAINER=false IMAGE_TAG=${LOKI_VERSION} loki \
    && go version -m cmd/loki/loki | tee /tmp/loki-build-info \
    && grep -F "golang.org/x/net v0.62.0" /tmp/loki-build-info

FROM gcr.io/distroless/static:nonroot
COPY --from=build /src/loki/cmd/loki/loki /usr/bin/loki
COPY --from=build /src/loki/cmd/loki/loki-docker-config.yaml /etc/loki/local-config.yaml
USER 10001
WORKDIR /
EXPOSE 3100
ENTRYPOINT ["/usr/bin/loki"]
CMD ["-config.file=/etc/loki/local-config.yaml"]
