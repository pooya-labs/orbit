# Keycloak 26.8.2 patched build experiment

This directory contains a source-build experiment and CI-only validation for the Guava CVE-2026-102554 present in the published Keycloak 26.8.2 image.

- Upstream source tag commit: `e514445f9dcdabc5bdcf67cab56c1e49655de067`.
- Quarkus baseline: 3.40.1.1.
- Override: import Guava BOM 33.7.2-jre before the Quarkus BOM, because the Quarkus 3.40.1.1 BOM manages Guava 33.6.0-jre.
- The patched workflow now builds and tests the complete candidate natively on both linux/amd64 and linux/arm64 using the `ubuntu-26.04` and `ubuntu-26.04-arm` runners. It verifies Maven dependency management, the generated distribution, full `/opt/keycloak` replacement, startup/readiness/OIDC, runtime library mappings, raw Trivy findings, and scoped-VEX output.
- Exact PR-head candidate matrix: [run 42537493841](https://github.com/pooya-labs/orbit/actions/runs/42537493841) passed both native jobs (amd64 and arm64). Both builds verified the Guava 33.7.2-jre distribution, replaced the complete `/opt/keycloak` tree, passed startup/readiness/OIDC and runtime-map checks, retained raw FTS5 detection, and passed the strict VEX-filtered HIGH/CRITICAL scan plus final assertions.

## FTS5 reachability evidence (2026-11-09)

- Exact Red Hat source RPM: `nss-3.124.0-5.el9_6.src.rpm`, SHA-256 `54413db2daff5780009e38d8e2ed0501b4e7a671302e09160de7f1990c52b4fe`. Its RPM signature verified with the official Red Hat release key fingerprint `567E347AD0044ADE55BA8A5F199E2F91FD431D51`; the package's declared downstream patches were applied with `rpmbuild -bp`.
- The resulting `lib/softoken` tree has no FTS5, `CREATE VIRTUAL TABLE`, `MATCH`, or SQLite extension-loading references. Its SQLite usage is regular table operations and parameterized statements.
- The published upstream Keycloak 26.8.2 image was started on native linux/amd64 and linux/arm64 runners. Readiness and OIDC discovery passed on both; no process in either container mapped `libsqlite3.so` or `libsoftokn3.so`.
- The exact-package OpenVEX statement for CVE-2026-119373 uses `vulnerable_code_not_in_execute_path` and applies only to the two sqlite-libs 3.34.1-11.el9_8 architecture-specific PURLs for this Keycloak 26.8.2 scan. It does not claim FTS5 is absent: the inspected amd64 library reports FTS5 compiled in.
- Deferred scan [run 42537493850](https://github.com/pooya-labs/orbit/actions/runs/42537493850) completed the ARM64 runtime probe and the upstream image scan. The strict scan filters the narrowly scoped SQLite/OpenJDK package findings; the sole remaining HIGH in the published upstream image is Guava 33.6.0-jre / CVE-2026-102554, whose fixed version is 33.7.2-jre. This expected upstream finding is not suppressed; the HIGH/CRITICAL gate and `ignore-unfixed: false` remain unchanged.

## Release boundary

This remains a CI-validated experiment, not a published production image or release artifact. The patched candidate passed native amd64 and arm64 builds, runtime checks and strict VEX-filtered scans in run 42537493841, but it has not been published. The production Compose file still points to upstream Keycloak 26.8.2, whose separate strict scan still reports the upstream Guava 33.6.0-jre HIGH. Do not switch production Compose or provision production identity until a reproducible multi-architecture candidate is published, its immutable digest is recorded, and strict HIGH/CRITICAL scans pass against that exact digest on both architectures.

Run `.github/workflows/keycloak-patched-build.yml`. Do not deploy or publish the candidate based on a successful single-architecture or Guava-only check.
