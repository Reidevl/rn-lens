"use strict";

/** Node's platform and arch names → the Go names GoReleaser uses. */
const GOOS = { darwin: "darwin", linux: "linux", win32: "windows" };
const GOARCH = { x64: "amd64", arm64: "arm64" };

/**
 * The npm package that carries the rnlens binary for this platform, or null
 * when Lens is not built for it.
 */
function platformPackage(platform = process.platform, arch = process.arch) {
  const os = GOOS[platform];
  const cpu = GOARCH[arch];
  if (!os || !cpu) return null;
  return {
    name: `@rnlens/cli-${os}-${cpu}`,
    binary: `bin/rnlens${platform === "win32" ? ".exe" : ""}`,
  };
}

/**
 * Where the rnlens binary is: RNLENS_BINARY when set, otherwise inside the
 * platform package npm installed as an optional dependency.
 */
function binaryPath({
  env = process.env,
  platform = process.platform,
  arch = process.arch,
  resolve = require.resolve,
} = {}) {
  if (env.RNLENS_BINARY) return env.RNLENS_BINARY;
  const pkg = platformPackage(platform, arch);
  if (!pkg) {
    throw new Error(
      `rnlens: no build for ${platform}/${arch}. Supported: macOS, Linux and Windows on x64 and arm64.`,
    );
  }
  try {
    return resolve(`${pkg.name}/${pkg.binary}`);
  } catch {
    throw new Error(
      `rnlens: ${pkg.name} is not installed. It is an optional dependency of rnlens; ` +
        "reinstall without --no-optional (or --omit=optional), or download rnlens from " +
        "https://github.com/Reidevl/rn-lens/releases.",
    );
  }
}

module.exports = { GOOS, GOARCH, platformPackage, binaryPath };
