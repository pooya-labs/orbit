FROM golang:1.27-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
ARG CADDY_COMMIT=72dd0fb067f6d7826c7f79907670ba4a713bfe37
RUN git init . \
    && git remote add origin https://github.com/caddyserver/caddy.git \
    && git fetch --depth=1 origin "${CADDY_COMMIT}" \
    && git checkout --detach FETCH_HEAD
RUN go get golang.org/x/net@v0.62.0 github.com/go-jose/go-jose/v4@v4.1.7 \
    && go mod tidy \
    && go mod verify
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -trimpath \
    -ldflags="-s -w -X github.com/caddyserver/caddy/v2.CustomVersion=2.11.7-orbit.1" \
    -o /out/caddy ./cmd

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 10001 -S caddy \
    && adduser -u 10001 -S caddy -G caddy \
    && mkdir -p /data /config /etc/caddy /srv \
    && chown -R 10001:10001 /data /config /srv
COPY --from=build /out/caddy /usr/bin/caddy
ENV XDG_DATA_HOME=/data \
    XDG_CONFIG_HOME=/config
WORKDIR /srv
USER 10001:10001
EXPOSE 80 443 443/udp
ENTRYPOINT ["/usr/bin/caddy"]
