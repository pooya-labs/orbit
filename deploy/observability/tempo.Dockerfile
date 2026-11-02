# Build the upstream Tempo v3.1.1 source with the patched go-jose dependency.
FROM golang:1.27-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
ARG TEMPO_COMMIT=78f057d3c089f29205cce898c3ecb44102d794ff
RUN git init . \
    && git remote add origin https://github.com/grafana/tempo.git \
    && git fetch --depth=1 origin "${TEMPO_COMMIT}" \
    && git checkout --detach FETCH_HEAD
RUN go mod edit -require=github.com/go-jose/go-jose/v4@v4.1.7 \
    && go mod tidy \
    && go mod verify
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -trimpath \
    -ldflags="-s -w -X main.Version=3.1.1" \
    -o /out/tempo ./cmd/tempo

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /var/tempo \
    && chown 10001:10001 /var/tempo
COPY --from=build /out/tempo /tempo
USER 10001:10001
EXPOSE 3200 4317
ENTRYPOINT ["/tempo"]
