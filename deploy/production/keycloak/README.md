# Keycloak 26.8.2 patched build experiment

This directory contains a source-build experiment and CI-only validation for the Guava CVE-2026-102554 present in the published Keycloak 26.8.2 image.

- Upstream source tag commit: `e514445f9dcdabc5bdcf67cab56c1e49655de067`.
- Quarkus baseline: 3.40.1.1.
- Override: import Guava BOM 33.7.2-jre before the Quarkus BOM, because the Quarkus 3.40.1.1 BOM manages Guava 33.6.0-jre.
- CI builds the upstream Quarkus distribution, verifies the effective Maven dependency management, checks the distribution contains only Guava 33.7.2-jre, replaces the complete `/opt/keycloak` tree in the Keycloak runtime base, starts the candidate, checks readiness and OIDC discovery, and compares raw Trivy findings with the scoped-VEX result.
- The candidate workflow is currently building the patched distribution on the PR head: [run 42526131049](https://github.com/pooya-labs/orbit/actions/runs/42526131049). This remains an isolated CI image; it is not published or deployed.

## FTS5 reachability evidence (2026-11-09)

- The exact Red Hat source RPM is `nss-3.124.0-5.el9_6.src.rpm`, SHA-256 `54413db2daff5780009e38d8e2ed0501b4e7a671302e09160de7f1990c52b4fe`. Its RPM signature verified with the official Red Hat release key fingerprint `567E347AD0044ADE55BA8A5F199E2F91FD431D51`; the package's declared downstream patches were applied with `rpmbuild -bp`.
- The resulting `lib/softoken` tree has no FTS5, `CREATE VIRTUAL TABLE`, `MATCH`, or SQLite extension-loading references. Its SQLite usage is regular table operations and parameterized statements.
- The published upstream Keycloak 26.8.2 image was started on both native linux/amd64 and linux/arm64 runners. Readiness and OIDC discovery passed on both; no process in either container mapped `libsqlite3.so` or `libsoftokn3.so`.
- The exact-package OpenVEX statement for CVE-2026-119373 uses `vulnerable_code_not_in_execute_path` and applies only to the two sqlite-libs 3.34.1-11.el9_8 architecture-specific PURLs for this Keycloak 26.8.2 scan. It does not claim FTS5 is absent: the inspected amd64 library reports FTS5 compiled in.
- Deferred scan [run 42525414926](https://github.com/pooya-labs/orbit/actions/runs/42525414926) confirms the VEX-filtered OS package section is clean on both architectures. The sole remaining HIGH in the upstream image is Guava 33.6.0-jre, for which the fixed version is 33.7.2-jre. The strict HIGH/CRITICAL gate and `ignore-unfixed: false` remain unchanged.
- The latest deferred identity run, [42526131111](https://github.com/pooya-labs/orbit/actions/runs/42526131111), repeats the signed source audit, VEX scan and ARM64 runtime probe on the current PR head.

## Release boundary

This is an experiment, not a production image or release artifact. The patched candidate has not yet been validated and scanned on both architectures or published. The production Compose file still points to upstream Keycloak 26.8.2. Do not switch production Compose or provision production identity until a reproducible multi-architecture candidate has passed build, runtime, raw-versus-VEX and exact-image strict-scan checks.

Run `.github/workflows/keycloak-patched-build.yml`. Do not deploy or publish the candidate based on a successful Guava-only check.
