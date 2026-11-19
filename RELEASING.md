# Releasing

For the maintainer. A release publishes the `rnlens` archives, the desktop
apps, the Homebrew cask, the Scoop manifest and the npm packages.

## Steps

1. Merge pull requests into `main` with Conventional Commit titles.
   Release Please keeps a pull request named `chore(main): release X.Y.Z`
   up to date with the version and the changelog.
2. Check that pull request (version, `CHANGELOG.md`), then merge it. Release
   Please tags the release and runs `.github/workflows/release.yml`.
3. When the workflow is green, **approve the staged npm versions**. The
   workflow stages them; none is live until you approve it with 2FA:
   - on npmjs.com, under **Staged Packages**, or
   - with `npm stage list` and `npm stage approve <stage-id>` (npm 11.15+).

   Approve all eight together: the six `@rnlens/cli-*` packages, `rnlens`
   and `@rnlens/sdk`. `rnlens` installs the matching `@rnlens/cli-*`
   package, so it only works once those are live too.

4. Check `npx rnlens@X.Y.Z --version`. npm can take about ten minutes before
   a new version downloads.

If a job fails:

- **The workflow is fine and the failure was transient** (a network error,
  a timeout): re-run only the failed jobs with `gh run rerun <run-id> --failed`.
  A full `gh workflow run` would also redo the GitHub release uploads.
- **The workflow needs a fix:** fix it on `main`, then run it again for the
  same tag with `gh workflow run release.yml -f tag=vX.Y.Z`. "Re-run failed
  jobs" reuses the old workflow file.

The npm jobs skip any version that is already published, so a re-run stages
only what is missing. npm refuses to stage a version that is already staged,
so approve (or reject) the staged ones before re-running.

## npm publishing setup

Each of the eight packages has **two Trusted Publishers** on npmjs.com, both
GitHub Actions on `Reidevl/rn-lens`:

| Workflow filename    | Covers                                                                                   |
| -------------------- | ---------------------------------------------------------------------------------------- |
| `release-please.yml` | Normal releases. npm checks the calling workflow, and Release Please calls `release.yml` |
| `release.yml`        | Runs started by hand with `gh workflow run release.yml`                                  |

Leave "Allow npm publish" and "Allow npm dist-tag" unchecked on both. Staging
is always allowed, so the publisher can stage versions but never put one live.

There is no npm token: every package's publishing access is "Require
two-factor authentication and disallow bypass 2fa tokens", and the workflows
authenticate through trusted publishing only.

## Secrets

| Secret                 | Used for                                           |
| ---------------------- | -------------------------------------------------- |
| `RELEASE_PLEASE_TOKEN` | Opening the release pull request as the maintainer |
| `HOMEBREW_TAP_TOKEN`   | Pushing the cask to `Reidevl/homebrew-tap`         |
| `SCOOP_BUCKET_TOKEN`   | Pushing the manifest to `Reidevl/scoop-bucket`     |

## Version files

Release Please bumps the files listed in `release-please-config.json`. The
`generic` ones carry `x-release-please-version` markers, and
`build/release_files_test.go` fails when a marker is missing or a marked
version differs from `.release-please-manifest.json`. `wails3 update
build-assets` (`task common:update:build-assets`) regenerates
`build/windows/nsis/wails_tools.nsh` and `build/windows/wails.exe.manifest`
without their markers: put them back afterwards.
