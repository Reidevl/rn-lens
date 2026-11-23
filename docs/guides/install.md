# Install

Lens comes in two builds of the same program:

- **`rnlens`** runs Lens in your browser ([browser mode](browser-mode.md)). It
  is one small binary for macOS, Linux and Windows, and what `npx`, Homebrew,
  Scoop and the release archives install.
- **The desktop app** runs Lens in its own window. It is attached to each
  GitHub release for macOS, Windows and Linux.

## npx

Needs Node.js 18 or newer. Nothing is installed globally:

```sh
npx rnlens
```

npm downloads only the binary for your platform (`@rnlens/cli-<os>-<arch>`)
and runs it. Pass flags as usual: `npx rnlens --port 9000 --no-open`.

## Homebrew (macOS, Linux)

```sh
brew install --cask reidevl/tap/rnlens
rnlens
```

## Scoop (Windows)

```sh
scoop bucket add reidevl https://github.com/Reidevl/scoop-bucket
scoop install rnlens
rnlens
```

## Release archives

Each [GitHub release](https://github.com/Reidevl/rn-lens/releases) has
`rnlens_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) for `darwin`,
`linux` and `windows` on `amd64` and `arm64`, where `<version>` has no
leading `v` (`rnlens_0.1.0_linux_amd64.tar.gz` for release `v0.1.0`). Unpack
it and run `rnlens`.

## Desktop app

Download the app for your OS from the release: `Lens_v<version>_macos_universal.zip`
(Apple silicon and Intel), `Lens_v<version>_windows_amd64_setup.exe`, or
`Lens_v<version>_linux_x86_64.AppImage`. Homebrew and Scoop install
`rnlens` (browser mode), not the desktop app.

**macOS:** the app is not notarized by Apple yet, so macOS says it "is
damaged" or "cannot be opened". After moving it to Applications, clear the
quarantine flag once:

```sh
xattr -dr com.apple.quarantine /Applications/Lens.app
```

The same applies to an `rnlens` binary you unpack by hand on macOS
(`xattr -d com.apple.quarantine ./rnlens`; it reports an error when the flag
is not set, which is harmless); Homebrew does it for you.

## Verify a download

Every release has `checksums.txt` (SHA-256 of each archive), its keyless
[cosign](https://docs.sigstore.dev/) signature, and GitHub build provenance.

```sh
# The archive matches the published checksum
shasum -a 256 --ignore-missing -c checksums.txt

# checksums.txt was signed by this repository's release workflow. It runs
# from main (release-please calls it after tagging) or by hand on a tag.
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp '^https://github\.com/Reidevl/rn-lens/\.github/workflows/release\.yml@refs/(heads/main|tags/v.+)$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# The archive was built by GitHub Actions from this repository
gh attestation verify rnlens_<version>_<os>_<arch>.tar.gz --repo Reidevl/rn-lens
```

Each archive also has an SPDX software bill of materials
(`<archive>.sbom.json`).
