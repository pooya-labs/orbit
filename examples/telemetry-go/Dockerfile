FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/orbit-telemetry-demo .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates
COPY --from=build /out/orbit-telemetry-demo /usr/local/bin/orbit-telemetry-demo

USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/orbit-telemetry-demo"]
