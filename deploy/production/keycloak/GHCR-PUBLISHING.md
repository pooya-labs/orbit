# GHCR publication — patched Keycloak 26.8.2

This document records ORBIT's owner-triggered Keycloak 26.8.2 publication. Publication is separate from deployment: the workflow publishes to GHCR and scans an immutable manifest digest, but it does not change production Compose, provision an identity service, or route traffic.

## Immutable build inputs

- Registry target: `ghcr.io/pooya-labs/orbit-keycloak`.
- Upstream base: `quay.io/keycloak/keycloak:26.8.2@sha256:723b1d6e254c0fc96d5cffec860c7e0744d6a3277822b865bb538cf891575e41`.
- Keycloak source commit: reconstructed and format-validated from the four checked fragments in the publisher workflow.
- Guava override: `33.7.2-jre`, applied through the reviewed patch in this directory.
- Native build matrix: `linux/amd64` on `ubuntu-26.04` and `linux/arm64` on `ubuntu-26.04-arm`.

The workflow verifies the pinned base manifest, source commit, Maven dependency resolution and generated distribution. It replaces the complete `/opt/keycloak` tree and tests startup, readiness, OIDC discovery and runtime mappings on each architecture.

## Publication gates

Run `.github/workflows/keycloak-ghcr-publish.yml` manually from `main` and explicitly set `confirm_publish=true`. Both native candidates must pass validation before architecture tags are pushed and a multi-architecture manifest is created.

For each candidate, the workflow runs a raw Trivy scan and a strict HIGH/CRITICAL scan using exact-scope OpenVEX statements. It requires raw visibility of CVE-2026-119373, CVE-2026-119374 and CVE-2026-119379, requires only the exact package/path VEX statements to filter those SQLite findings, and requires Guava CVE-2026-102554 to be absent. Trivy cache is disabled and `ignore-unfixed=false`.

## Published artifact — 2026-11-09

**Published and verified**

- Immutable image reference: `ghcr.io/pooya-labs/orbit-keycloak@sha256:97cc4590f5abd77e23522d6d94c3fe77c82b91c44a4e7acef22aec28c61bf81d`.
- Commit-specific tag: `ghcr.io/pooya-labs/orbit-keycloak:26.8.2-guava-33.7.2-c4e45b9c1f0be4fd878c32366c7743a28d079227`.
- Manifest platforms: `linux/amd64` and `linux/arm64`.
- Publication workflow: [run 42654381960, attempt 2](https://github.com/pooya-labs/orbit/actions/runs/42654381960).
- Native AMD64 and ARM64 build, runtime validation, candidate scans and pushes: success.
- Exact published-digest pull and strict scan on AMD64: success.
- Exact published-digest pull and strict scan on ARM64: success.

Both digest scans confirmed raw visibility of the expected SQLite findings, scoped VEX filtering of only those package/path findings, and absence of patched Guava CVE-2026-102554.

## Package access and production boundary

The workflow uses the repository-scoped `GITHUB_TOKEN`; no PAT or new registry secret is required. Repository `pooya-labs/orbit` has **Write** access under the package's **Manage Actions access** settings. This work did not change the package's public/private visibility; any unauthenticated-pull policy must be chosen separately.

Production Compose remains unchanged. This workflow did not deploy the image, provision an identity service, modify DNS/routing, or contact live tenant traffic. Any future production promotion requires separate approval and must pin `ghcr.io/pooya-labs/orbit-keycloak@sha256:97cc4590f5abd77e23522d6d94c3fe77c82b91c44a4e7acef22aec28c61bf81d`, not a mutable tag.
