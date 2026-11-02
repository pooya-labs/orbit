# GitHub Pages deployment

ORBIT is a Next.js static export deployed from the `out/` artifact by
`.github/workflows/deploy.yml`. The repository must use the **GitHub Actions**
publishing source. Branch-based Jekyll publishing is not compatible with this
build.

## One-time repository setting

1. Open [ORBIT repository settings](https://github.com/pooya16pg-spec/orbit/settings/pages).
2. Under **Build and deployment**, set **Source** to **GitHub Actions**.
3. Save the setting.
4. Rerun the **Deploy ORBIT to GitHub Pages** workflow from the Actions tab, or
   push a commit to `main`.

Both the pull-request verification workflow and the deployment workflow read
the repository's Pages `build_type` from GitHub. They require `workflow` and
fail with an actionable message when the source is `legacy` or unknown. This
prevents an Actions deployment from silently racing a second branch/Jekyll
deployment derived from repository files such as README.md.

The source is a repository setting, not an application file. The workflow
checks it but does not attempt to change repository settings with a deployment
token.

## Published URL

The project site is:

https://pooya16pg-spec.github.io/orbit/

The repository root domain (without `/orbit/`) is not the URL for this
project site.
