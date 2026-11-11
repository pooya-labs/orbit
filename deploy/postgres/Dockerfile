# Rebuild the upstream gosu helper with a patched Go toolchain.
# The pinned upstream source tag is gosu v1.20 (commit 29b939aa5bd4dc588af765b8248f6e39150d2565).
# This replaces only /usr/local/bin/gosu; PostgreSQL and its official entrypoint remain upstream.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS gosu-build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG GOSU_COMMIT=29b939aa5bd4dc588af765b8248f6e39150d2565

RUN apk add --no-cache git
WORKDIR /src
RUN git init . \
    && git remote add origin https://github.com/tianon/gosu.git \
    && git fetch --depth=1 origin "${GOSU_COMMIT}" \
    && git checkout --detach FETCH_HEAD

RUN go mod verify
RUN set -eux; \
    export GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-amd64}"; \
    if [ "$TARGETARCH" = "arm" ] && [ -n "$TARGETVARIANT" ]; then export GOARM="${TARGETVARIANT#v}"; fi; \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/gosu .

FROM postgres:17-alpine
COPY --from=gosu-build /out/gosu /usr/local/bin/gosu

# The official image may predate security updates published in its Alpine branch.
# Refresh final-stage OS packages so fixed repository builds are present in the
# runtime image scanned by CI, without changing PostgreSQL or its entrypoint.
RUN apk upgrade --no-cache \
    && gosu --version \
    && gosu nobody true

# Optional Keycloak database, created only on a fresh production volume.
COPY --chmod=755 deploy/postgres/init/10-keycloak-db.sh /docker-entrypoint-initdb.d/10-keycloak-db.sh
