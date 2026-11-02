FROM golang:1.27-alpine AS build
WORKDIR /src
COPY scripts/telemetry-smoke/main.go ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/telemetry-smoke .

FROM scratch
COPY --from=build /out/telemetry-smoke /telemetry-smoke
USER 65532:65532
ENTRYPOINT ["/telemetry-smoke"]
