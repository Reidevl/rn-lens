# rnlens

Runs [Lens](https://github.com/Reidevl/rn-lens), React Native DevTools, in your
browser:

```sh
npx rnlens
```

It opens `http://localhost:7430`. Flags pass through:
`npx rnlens --port 9000 --no-open`. See the
[install guide](https://github.com/Reidevl/rn-lens/blob/main/docs/guides/install.md)
and [browser mode](https://github.com/Reidevl/rn-lens/blob/main/docs/guides/browser-mode.md).

## How it works

This package is a small launcher. npm installs one of the
`@rnlens/cli-<os>-<arch>` optional dependencies, the one that matches your
machine, and the launcher runs the binary inside it. Set `RNLENS_BINARY` to
run another binary instead.

## For maintainers

The packages are generated from a GoReleaser build, never edited by hand:

```sh
goreleaser release --snapshot --clean      # or the release workflow's dist/
node packages/rnlens/scripts/build-npm.js --root . --out dist/npm --version v0.1.0
```

`dist/npm` then holds `rnlens` and every `cli-<os>-<arch>` package at the same
version. Publish the platform packages before the launcher.
