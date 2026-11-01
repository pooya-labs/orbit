# GitHub Pages deployment

ORBIT is a Next.js static export deployed from the `out/` artifact by
`.github/workflows/deploy.yml`. The repository must use the **GitHub Actions**
publishing source; branch-based Jekyll publishing is not compatible with this
build.

## One-time repository setting

1. Open [ORBIT repository settings](https://github.com/pooya16pg-spec/orbit/settings/pages).
2. Under **Build and deployment**, set **Source** to **GitHub Actions**.
3. Save the setting.
4. Rerun the **Deploy ORBIT to GitHub Pages** workflow from the Actions tab, or
   push a commit to `main`.

The workflow checks the Pages `build_type` before building. It stops with an
actionable error when the source is still `legacy`, because GitHub would also
run its branch/Jekyll publisher and that publisher can replace the Next.js
artifact with a README-derived page.

## Published URL

The project site is:

https://pooya16pg-spec.github.io/orbit/

The repository root domain (without `/orbit/`) is not the URL for this
project site.
