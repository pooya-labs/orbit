#!/usr/bin/env bash
set -euo pipefail

tmp_root="${RUNNER_TEMP:-/tmp}"
workdir="${tmp_root%/}/orbit-nss-source-audit"
srpm_name="nss-3.124.0-5.el9_6.src.rpm"
srpm_url="https://cdn-ubi.redhat.com/content/public/ubi/dist/ubi9/9/x86_64/appstream/source/SRPMS/Packages/n/${srpm_name}"
report="${tmp_root%/}/nss-sqlite-source-audit.txt"

rm -rf "$workdir"
mkdir -p "$workdir/srpm" "$workdir/rpmbuild"/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

sudo apt-get update
sudo apt-get install -y --no-install-recommends rpm rpm2cpio cpio

printf 'Downloading exact Red Hat NSS source RPM: %s\n' "$srpm_url"
curl --fail --location --retry 3 --retry-all-errors --connect-timeout 30 --max-time 900 \
  "$srpm_url" --output "$workdir/srpm/$srpm_name"

actual_nevra="$(rpm -qp --qf '%{NAME}-%{VERSION}-%{RELEASE}' "$workdir/srpm/$srpm_name")"
if [[ "$actual_nevra" != "nss-3.124.0-5.el9_6" ]]; then
  echo "Unexpected source RPM identity: $actual_nevra" >&2
  exit 1
fi

{
  echo "ORBIT NSS / SQLite FTS5 source audit"
  echo "Source RPM: $actual_nevra"
  echo "Downloaded SHA-256: $(sha256sum "$workdir/srpm/$srpm_name" | awk '{print $1}')"
  echo "Source URL: $srpm_url"
  echo
  echo "== RPM signature metadata =="
  rpm -Kv "$workdir/srpm/$srpm_name" 2>&1 || true
} > "$report"

(
  cd "$workdir/srpm"
  rpm2cpio "$srpm_name" | cpio --extract --make-directories --no-absolute-filenames --quiet
)

spec_file="$(find "$workdir/srpm" -maxdepth 1 -type f -name '*.spec' -print -quit)"
if [[ -z "$spec_file" ]]; then
  echo "The exact source RPM did not contain nss.spec" | tee -a "$report" >&2
  exit 1
fi

{
  echo
  echo "== Source and patch declarations from nss.spec =="
  grep -nE '^(Name|Version|Release|Source[0-9]*|Patch[0-9]*):' "$spec_file" || true
} >> "$report"

topdir="$workdir/rpmbuild"
cp "$spec_file" "$topdir/SPECS/nss.spec"
while IFS= read -r -d '' source_file; do
  cp "$source_file" "$topdir/SOURCES/"
done < <(find "$workdir/srpm" -maxdepth 1 -type f ! -name '*.spec' -print0)

echo "Applying the exact source RPM's declared patches with rpmbuild -bp..."
if ! rpmbuild --define "_topdir $topdir" --define "__spec_prep_shell /bin/bash" -bp --nodeps "$topdir/SPECS/nss.spec" \
  > "$workdir/rpmbuild-prep.log" 2>&1; then
  {
    echo
    echo "== rpmbuild -bp failed; diagnostic output =="
    tail -n 240 "$workdir/rpmbuild-prep.log"
  } >> "$report"
  cat "$report"
  exit 1
fi

softoken_dir="$(find "$topdir/BUILD" -type d -path '*/lib/softoken' -print -quit)"
if [[ -z "$softoken_dir" ]]; then
  echo "The prepared NSS source tree has no lib/softoken directory" | tee -a "$report" >&2
  exit 1
fi

{
  echo
  echo "== Prepared NSS source tree =="
  echo "Softoken source: $softoken_dir"
  echo
  echo "== FTS-related source references in lib/softoken =="
  if grep -RInE --include='*.c' --include='*.h' --include='*.inc' \
    'fts5|CREATE[[:space:]]+VIRTUAL[[:space:]]+TABLE|[[:space:]]MATCH[[:space:]]' "$softoken_dir"; then
    echo "FTS_SOURCE_REFERENCES_FOUND=1"
  else
    echo "FTS_SOURCE_REFERENCES_FOUND=0"
  fi
  echo
  echo "== SQLite API call sites in lib/softoken (first 240 matches) =="
  grep -RInE --include='*.c' --include='*.h' \
    'sqlite3_(open|prepare|exec|step|bind|create_function|load_extension)' "$softoken_dir" \
    | sed -n '1,240p' || true
  echo
  echo "== Explicit dynamic extension loading references =="
  grep -RInE --include='*.c' --include='*.h' \
    'sqlite3_load_extension|sqlite3_enable_load_extension' "$softoken_dir" || true
  echo
  echo "This report is source-level evidence only. It does not establish runtime reachability or justify a VEX statement."
} >> "$report"

cat "$report"
