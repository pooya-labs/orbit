FROM golang:1.27-alpine AS build
RUN go install go.opentelemetry.io/collector/cmd/builder@v0.165.0
WORKDIR /build
COPY deploy/observability/collector-builder.yaml ./builder.yaml
RUN builder --config=builder.yaml

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /build/_build/orbit-otel-collector /orbit-otel-collector
USER 10001:10001
EXPOSE 4317 4318 13133 9464
ENTRYPOINT ["/orbit-otel-collector"]
CMD ["--config", "/etc/otelcol-contrib/config.yaml"]
