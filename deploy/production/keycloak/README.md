# Keycloak 26.8.2 patched build and published artifact

This directory documents the reproducible Keycloak 26.8.2 source build, scoped SQLite reachability evidence, and the validated multi-architecture GHCR artifact. The patched distribution replaces vulnerable Guava 33.6.0-jre with 33.7.2-jre.

- Upstream source tag commit: `e514445f9dcdabc5bdcf67cab56c1e49655de067`.
- Quarkus baseline: 3.40.1.1.
- Override: import Guava BOM 33.7.2-jre before the Quarkus BOM, because the Quarkus 3.40.1.1 BOM manages Guava 33.6.0-jre.
- The patched workflow now builds and tests the complete candidate natively on both linux/amd64 and linux/arm64 using the `ubuntu-26.04` and `ubuntu-26.04-arm` runners. It verifies Maven dependency management, the generated distribution, full `/opt/keycloak` replacement, startup/readiness/OIDC, runtime library mappings, raw Trivy findings, and scoped-VEX output.
- Exact PR-head candidate matrix: [run 42537493841](https://github.com/pooya-labs/orbit/actions/runs/42537493841) passed both native jobs (amd64 and arm64). Both builds verified the Guava 33.7.2-jre distribution, replaced the complete `/opt/keycloak` tree, passed startup/readiness/OIDC and runtime-map checks, retained raw FTS5 detection, and passed the strict VEX-filtered HIGH/CRITICAL scan plus final assertions.

## Published multi-architecture artifact — 2026-11-09

The owner-triggered GHCR publication completed successfully in [workflow run 42654381960, attempt 2](https://github.com/pooya-labs/orbit/actions/runs/42654381960).

- Image: `ghcr.io/pooya-labs/orbit-keycloak@sha256:97cc4590f5abd77e23522d6d94c3fe77c82b91c44a4e7acef22aec28c61bf81d`
- Immutable commit tag: `ghcr.io/pooya-labs/orbit-keycloak:26.8.2-guava-33.7.2-c4e45b9c1f0be4fd878c32366c7743a28d079227`
- Platforms: `linux/amd64` and `linux/arm64`.
- Both native candidate build, readiness/OIDC/runtime, SQLite compile-option and local strict scan jobs passed.
- The workflow pulled this exact manifest digest separately on native AMD64 and ARM64 runners. Both strict published-digest scans passed with `ignore-unfixed=false`.
- The raw reports retained the expected FTS5, FTS5 secure-delete and SQLite Session findings; the exact-package/path OpenVEX statements filtered only those scoped findings, and Guava CVE-2026-102554 was absent.

The GHCR publication proves the patched artifact is available by immutable digest. It does not deploy the image or change production Compose.

## FTS5 reachability evidence (2026-11-09)

- Exact Red Hat source RPM: `nss-3.124.0-5.el9_6.src.rpm`, SHA-256 `54413db2daff5780009e38d8e2ed0501b4e7a671302e09160de7f1990c52b4fe`. Its RPM signature verified with the official Red Hat release key fingerprint `567E347AD0044ADE55BA8A5F199E2F91FD431D51`; the package's declared downstream patches were applied with `rpmbuild -bp`.
- The resulting `lib/softoken` tree has no FTS5, `CREATE VIRTUAL TABLE`, `MATCH`, or SQLite extension-loading references. Its SQLite usage is regular table operations and parameterized statements.
- The published upstream Keycloak 26.8.2 image was started on native linux/amd64 and linux/arm64 runners. Readiness and OIDC discovery passed on both; no process in either container mapped `libsqlite3.so` or `libsoftokn3.so`.
- The exact-package OpenVEX statement for CVE-2026-119373 uses `vulnerable_code_not_in_execute_path` and applies only to the two sqlite-libs 3.34.1-11.el9_8 architecture-specific PURLs for this Keycloak 26.8.2 scan. It does not claim FTS5 is absent: the inspected amd64 library reports FTS5 compiled in.
- Deferred scan [run 42537493850](https://github.com/pooya-labs/orbit/actions/runs/42537493850) completed the ARM64 runtime probe and the upstream image scan. The strict scan filters the narrowly scoped SQLite/OpenJDK package findings; the sole remaining HIGH in the published upstream image is Guava 33.6.0-jre / CVE-2026-102554, whose fixed version is 33.7.2-jre. This expected upstream finding is not suppressed; the HIGH/CRITICAL gate and `ignore-unfixed: false` remain unchanged.

## Release boundary

The patched multi-architecture image is now published and its immutable digest has passed strict HIGH/CRITICAL scans on both native architectures. It is a validated registry artifact, not a production deployment. Production Compose, identity provisioning, DNS/routing and live tenant traffic remain unchanged. Any future production promotion requires a separate explicit deployment decision and must pin the immutable digest above rather than a mutable tag.

The source-build validation remains reproducible through `.github/workflows/keycloak-patched-build.yml`; controlled publication is performed by `.github/workflows/keycloak-ghcr-publish.yml` with `confirm_publish=true`.


## SQLite FTS5 and Session path review — 9 November 2026

The current upstream Keycloak 26.8.2 scan also reports HIGH findings CVE-2026-119374 (FTS5 secure-delete) and CVE-2026-119379 (SQLite Session changesets) against the exact `sqlite-libs 3.34.1-11.el9_8` RPM PURLs on both architectures. These CVEs are not treated as fixed or absent from the library.

Both compile options are enabled (`SQLITE_ENABLE_FTS5=1`, `SQLITE_ENABLE_SESSION=1`) on the inspected image. The signed and patched Red Hat NSS 3.124.0-5.el9_6 source audit now explicitly checks for FTS5 secure-delete references and SQLite Session/changeset APIs; results are `FTS_SOURCE_REFERENCES_FOUND=0`, `FTS5_SECURE_DELETE_REFERENCES_FOUND=0`, and `SQLITE_SESSION_API_REFERENCES_FOUND=0`. Native AMD64/ARM64 Keycloak runtime probes passed readiness and OIDC discovery and found no process mapping either `libsqlite3.so` or `libsoftokn3.so`.

The two new OpenVEX statements therefore use only `vulnerable_code_not_in_execute_path`, scoped to the two exact `sqlite-libs` PURLs in the Keycloak 26.8.2 scan. Raw Trivy findings remain visible; the strict filtered scan must prove the matching statements apply. These statements are not general SQLite exemptions and must be revalidated after changes to the image, NSS/SQLite packages, runtime or crypto/provider configuration.

Evidence: deferred identity workflow [run 42640592664](https://github.com/pooya-labs/orbit/actions/runs/42640592664), source audit [artifact 13684951865](https://github.com/pooya-labs/orbit/actions/runs/42640592664/artifacts/13684951865), AMD64 runtime mapping [artifact 13684747105](https://github.com/pooya-labs/orbit/actions/runs/42640592664/artifacts/13684747105), and ARM64 runtime mapping [artifact 13683214768](https://github.com/pooya-labs/orbit/actions/runs/42640592664/artifacts/13683214768).
