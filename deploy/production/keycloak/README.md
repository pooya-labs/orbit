# Keycloak 26.8.2 patched build experiment

This directory contains an upstream-source patch and CI-only validation for the Guava CVE-2026-102554 present in the published Keycloak 26.8.2 image.

- Upstream source tag commit: `e514445f9dcdabc5bdcf67cab56c1e49655de067`.
- Quarkus baseline: 3.40.1.1.
- Override: import Guava BOM 33.7.2-jre before the Quarkus BOM, because the Quarkus 3.40.1.1 BOM manages Guava 33.6.0-jre.
- CI builds the upstream Quarkus distribution and replaces the complete `/opt/keycloak` tree in the Keycloak runtime base with that generated distribution (it does not layer the new JARs over the old library directory). It then starts Keycloak, checks management readiness and OIDC discovery, and scans the candidate with the repository-pinned Trivy Action.
- Candidate workflow [run 42520290804](https://github.com/pooya-labs/orbit/actions/runs/42520290804) passed its amd64 source-build, runtime and initial Trivy checks. Follow-up CI now compares a raw Trivy scan with the narrowly scoped VEX-filtered scan.
- An exact Red Hat NSS source RPM audit found no FTS5 virtual-table, MATCH or SQLite extension-loading path in the patched NSS Softoken consumer. Runtime inventory found `libsoftokn3.so` as the only inspected native object linked to libsqlite3. OpenVEX covers CVE-2026-119373 only for the exact sqlite-libs x86_64/aarch64 package PURLs in Keycloak 26.8.2, with justification `vulnerable_code_not_in_execute_path`. FTS5 is compiled into the library; the VEX is based on the consumer path, not absence of library code.
- Deferred identity scan [run 42525007580](https://github.com/pooya-labs/orbit/actions/runs/42525007580) filters the scoped SQLite findings on both architectures; upstream Guava remains the sole HIGH blocker.

This is an experiment, not a production image or release artifact. The patched candidate has not yet been validated on both architectures or published. The production Compose file still points to the upstream image. Do not switch production Compose until the patched multi-architecture image passes the raw-versus-VEX test, readiness and OIDC smoke checks, and exact-image strict scans.

Run `.github/workflows/keycloak-patched-build.yml`. Do not deploy or publish the candidate based on a successful Guava-only check.
