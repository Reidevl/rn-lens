// Metro configuration for a monorepo example.
//
// `@rnlens/sdk` is installed as a symlink to ../../packages/sdk, and Metro
// follows symlinks to their real path. From there a hierarchical lookup would
// find the SDK's own devDependencies (packages/sdk/node_modules/react, ...)
// and bundle a second copy of React. The resolver below sends the SDK's peer
// dependencies back to this app's node_modules so every package resolves once.
const fs = require("node:fs");
const path = require("node:path");

const { getDefaultConfig } = require("expo/metro-config");

const projectRoot = __dirname;
// Real path: Metro reports real paths in originModulePath, so a symlinked
// checkout must compare against the resolved location.
const sdkRoot = fs.realpathSync(path.resolve(projectRoot, "../../packages/sdk"));

/** Peers the SDK must share with the app (never a second copy). */
const SHARED_PEERS = new Set(["react", "react-native", "@tanstack/react-query"]);

function packageRoot(moduleName) {
  const parts = moduleName.split("/");
  return moduleName.startsWith("@") ? parts.slice(0, 2).join("/") : parts[0];
}

const config = getDefaultConfig(projectRoot);

config.watchFolders = [sdkRoot];
config.resolver.blockList = [
  new RegExp(`${sdkRoot.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/node_modules/.*`),
];
config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (context.originModulePath.startsWith(sdkRoot) && SHARED_PEERS.has(packageRoot(moduleName))) {
    const fromApp = { ...context, originModulePath: path.join(projectRoot, "App.tsx") };
    return context.resolveRequest(fromApp, moduleName, platform);
  }
  return context.resolveRequest(context, moduleName, platform);
};

module.exports = config;
