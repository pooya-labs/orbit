# Keycloak 26.8.2 patched build experiment

This directory contains an upstream-source patch and CI-only validation for the Guava CVE-2026-102554 present in the published Keycloak 26.8.2 image.

- Upstream source tag commit: `e514445f9dcdabc5bdcf67cab56c1e49655de067`.
- Quarkus baseline: 3.40.1.1.
- Override: import Guava BOM 33.7.2-jre before the Quarkus BOM, because the Quarkus 3.40.1.1 BOM manages Guava 33.6.0-jre.
- CI builds the upstream Quarkus distribution, overlays it onto the upstream Keycloak runtime base, starts Keycloak, checks management readiness and OIDC discovery, then scans the candidate.

This is an experiment, not a production image or release artifact. The upstream Keycloak image remains pinned in production Compose until build, runtime smoke and both-architecture strict scans pass. SQLite FTS5 CVE-2026-119373 remains a separate blocking finding and is intentionally not suppressed.

Run `.github/workflows/keycloak-patched-build.yml`. Do not deploy or publish the candidate based on a successful Guava-only check.
