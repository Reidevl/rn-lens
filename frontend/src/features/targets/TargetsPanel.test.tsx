import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import {
  TOPICS,
  createMockBackend,
  idleSession,
  sampleTarget,
  type SessionSnapshot,
} from "@/backend";

import { useTargetsStore } from "./targetsStore";

describe("TargetsPanel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "targets" });
    useTargetsStore.setState({ targets: [], session: idleSession, error: null, busy: false });
  });

  it("shows an empty state until Metro lists an app", async () => {
    render(<App backend={createMockBackend()} />);
    expect(await screen.findByText(/No React Native app found yet/)).toBeInTheDocument();
  });

  it("lists discovered apps after a targets:changed notification", async () => {
    const backend = createMockBackend();
    render(<App backend={backend} />);
    await screen.findByText(/No React Native app found yet/);

    backend.setTargets([
      sampleTarget({
        capabilities: {
          nativePageReloads: true,
          nativeSourceCodeFetching: false,
          supportsMultipleDebuggers: false,
        },
      }),
    ]);

    expect(await screen.findByText("host.exp.Exponent (iPhone 17)")).toBeInTheDocument();
    expect(screen.getByText("exclusive attach")).toBeInTheDocument();
  });

  it("attaches, shows metadata and reloads", async () => {
    const user = userEvent.setup();
    const target = sampleTarget();
    const backend = createMockBackend({ targets: [target] });
    render(<App backend={backend} />);

    await user.click(await screen.findByRole("button", { name: /attach to host.exp.Exponent/i }));
    await waitFor(() => expect(backend.calls).toContain(`attach:${target.id}`));

    backend.setSession({
      ...idleSession,
      state: "attached",
      target,
      metadata: {
        appDisplayName: "Lens Example",
        appIdentifier: "host.exp.Exponent",
        deviceName: "iPhone 17",
        integrationName: "iOS Bridgeless (RCTHost)",
        platform: "ios",
        reactNativeVersion: "0.88.0",
        unstable_isProfilingBuild: false,
        unstable_networkInspectionEnabled: true,
        unstable_frameRecordingEnabled: false,
        unstable_networkThrottlingEnabled: false,
      },
    });

    expect(await screen.findByText("Connected")).toBeInTheDocument();
    expect(screen.getByText("0.88.0")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Lens Example · ios · RN 0.88.0");

    await user.click(screen.getByRole("button", { name: "Reload app" }));
    await waitFor(() => expect(backend.calls).toContain("reload"));
  });

  it("explains an eviction and offers to attach again", async () => {
    const target = sampleTarget();
    const backend = createMockBackend({
      targets: [target],
      session: {
        ...idleSession,
        state: "evicted",
        target,
        reason: "[NEW_DEBUGGER_OPENED] New debugger opened",
      },
    });
    render(<App backend={backend} />);

    // Shown twice on purpose: in the session card and in the status bar.
    expect(await screen.findAllByText("Disconnected by another debugger")).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Attach again" })).toBeInTheDocument();
  });

  it("keeps the newest session when an older pull resolves last", async () => {
    const target = sampleTarget();
    const backend = createMockBackend({ targets: [target] });
    // Every session() call parks here until the test answers it, so pulls can
    // be resolved out of order.
    const pending: Array<(session: SessionSnapshot) => void> = [];
    backend.targets.session = () =>
      new Promise<SessionSnapshot>((resolve) => {
        pending.push(resolve);
      });
    const answer = (snapshot: SessionSnapshot) => {
      const resolve = pending.shift();
      if (!resolve) throw new Error("no pending session() call");
      resolve(snapshot);
    };
    render(<App backend={backend} />);

    await waitFor(() => expect(pending).toHaveLength(1));
    answer({ ...idleSession, state: "attached", target });
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent(target.title));

    // Two overlapping pulls, A then B; Go answers B (closed) before A (attached).
    backend.emit(TOPICS.sessionChanged, { topic: TOPICS.sessionChanged, seq: 1 });
    backend.emit(TOPICS.sessionChanged, { topic: TOPICS.sessionChanged, seq: 2 });
    await waitFor(() => expect(pending).toHaveLength(2));
    const answerA = pending.shift();
    answer({ ...idleSession, state: "closed", reason: "detached" });
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Disconnected"));

    answerA?.({ ...idleSession, state: "attached", target });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(screen.getByRole("status")).toHaveTextContent("Disconnected");
    expect(
      screen.getByRole("button", { name: /attach to host.exp.Exponent/i }),
    ).toBeInTheDocument();
  });

  it("surfaces action errors inline", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend({ targets: [sampleTarget()] });
    backend.targets.reload = () => Promise.reject(new Error("targets: no attached session"));
    backend.targets.attach = () => Promise.reject(new Error("app: unknown target"));
    render(<App backend={backend} />);

    await user.click(await screen.findByRole("button", { name: /attach to/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent("app: unknown target");
  });
});
