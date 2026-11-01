FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod tidy
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/orbit-api ./cmd/orbit-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/orbit-api /orbit-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/orbit-api"]
