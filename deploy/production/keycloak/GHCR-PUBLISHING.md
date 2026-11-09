# GHCR publication — patched Keycloak 26.8.2

This document covers the owner-triggered publication of the patched Keycloak 26.8.2 image built by ORBIT. Publication is separate from deployment: the workflow publishes to GHCR and scans the immutable manifest digest, but it does not change production Compose, provision an identity service, or route traffic.

## Immutable build inputs

- Registry target: `ghcr.io/pooya-labs/orbit-keycloak`.
- Upstream base image: `quay.io/keycloak/keycloak:26.8.2@sha256:723b1d6e254c0fc96d5cffec860c7e0744d6a3277822b865bb538cf891575e41`.
- Keycloak source commit: stored as four checked fragments in the publisher workflow (`e514445f9d`, `cdabc5bdcf`, `67cab56c1e`, `49655de067`) and reconstructed/format-validated before fetch.
- Guava override: `33.7.2-jre`, applied through the reviewed patch in this directory.
- Native build matrix: `linux/amd64` on `ubuntu-26.04` and `linux/arm64` on `ubuntu-26.04-arm`.

The workflow verifies the pinned base manifest contains both architectures, checks the reconstructed source commit before checkout, verifies Maven dependency management and the generated distribution, replaces the complete `/opt/keycloak` tree, and tests startup, readiness, OIDC discovery and runtime mappings on each architecture.

## Publication gates

Run `.github/workflows/keycloak-ghcr-publish.yml` manually from `main` and explicitly set `confirm_publish=true`. Both architecture builds must complete their checks before the manifest job can run.

For each local candidate, the workflow runs a raw Trivy scan with no VEX references and a strict HIGH/CRITICAL scan using the five exact-scope OpenVEX statements. Its assertions require raw visibility of all three SQLite findings — CVE-2026-119373, CVE-2026-119374 and CVE-2026-119379 — require all three to be removed only by their exact package/path VEX statements, and require Guava CVE-2026-102554 to be absent. Trivy cache is disabled and `ignore-unfixed=false`.

Only after both native candidates pass are their architecture-specific tags pushed. The workflow then creates a commit-specific multi-architecture manifest, records its immutable digest, and repeats the raw and strict scans against that exact published digest on native AMD64 and ARM64 runners. The published-digest assertion repeats the three-SQLite-finding and patched-Guava checks.

## Package access and production boundary

The workflow uses the repository-scoped `GITHUB_TOKEN`; it does not require a PAT or a new registry secret. A newly created GHCR container package is private by default. After the exact digest passes both strict scans, the owner must deliberately choose whether the package should become public for unauthenticated pulls or remain private with authenticated pulls configured.

Do not switch production Compose or provision production identity until the exact published digest has passed both architecture scans and the package-access decision is complete. Any later production change must pin `ghcr.io/pooya-labs/orbit-keycloak@sha256:...`, not a mutable tag.

No image is published by a pull request or by CI validation alone. The manual publication workflow has not been run by these PR changes, no GHCR package has been created, and production Compose remains unchanged.
