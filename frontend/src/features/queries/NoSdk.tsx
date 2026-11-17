import { useEffect, useState } from "react";

import { useBackend } from "@/backend";

/** Shown when no app has connected through @rnlens/sdk yet. */
export function NoSdk() {
  const backend = useBackend();
  const [addr, setAddr] = useState("");
  useEffect(() => {
    backend.queries
      .sdkAddr()
      .then(setAddr)
      .catch(() => {});
  }, [backend]);

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-sm text-muted-foreground">
      <p className="font-medium text-foreground">No app has connected through the Lens SDK yet.</p>
      <p>Install the SDK and mount the hook next to your QueryClientProvider:</p>
      <pre className="rounded-md border border-border bg-card p-3 font-mono text-xs text-foreground select-text">
        {`npm install @rnlens/sdk

import { useLens } from "@rnlens/sdk";

function App() {
  useLens({ queryClient });
  return <QueryClientProvider client={queryClient}>…</QueryClientProvider>;
}`}
      </pre>
      <p>
        Lens listens on{" "}
        <code className="font-mono">{addr ? `ws://${addr}/lens` : "port 7420"}</code>. The SDK is a
        no-op in production builds.
      </p>
    </div>
  );
}
