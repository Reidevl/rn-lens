import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import type { LensMessage } from "@rnlens/protocol";
import Ajv2020 from "ajv/dist/2020";
import addFormats from "ajv-formats";
import { WebSocketServer, type WebSocket as ServerSocket } from "ws";

const schemaPath = resolve(__dirname, "../../../../protocol/schema/lens-protocol.v1.schema.json");
const schema = JSON.parse(readFileSync(schemaPath, "utf8"));
const ajv = new Ajv2020({ strict: true, allErrors: true });
addFormats(ajv);
export const validateFrame = ajv.compile(schema);

type Waiter = { type: LensMessage["type"]; resolve: (m: LensMessage) => void };

/**
 * Fake Lens server for tests: records every inbound frame, validates each one
 * against the protocol schema and lets a test push frames to the device.
 */
export class FakeLens {
  readonly frames: LensMessage[] = [];
  readonly invalid: Array<{ frame: unknown; errors: unknown }> = [];
  readonly url: string;
  private readonly sockets = new Set<ServerSocket>();
  private readonly queues = new Map<string, LensMessage[]>();
  private readonly waiters: Waiter[] = [];
  private readonly connections: Array<() => void> = [];

  private constructor(
    private readonly server: WebSocketServer,
    port: number,
  ) {
    this.url = `ws://127.0.0.1:${port}/lens`;
    server.on("connection", (socket) => {
      this.sockets.add(socket);
      socket.on("close", () => this.sockets.delete(socket));
      socket.on("message", (raw) => this.receive(raw.toString()));
      for (const cb of this.connections.splice(0)) cb();
    });
  }

  static start(): Promise<FakeLens> {
    return new Promise((resolvePromise, reject) => {
      const server = new WebSocketServer({ host: "127.0.0.1", port: 0, path: "/lens" });
      server.once("error", reject);
      server.once("listening", () => {
        const address = server.address();
        if (typeof address === "string" || address === null) {
          reject(new Error("unexpected address"));
          return;
        }
        resolvePromise(new FakeLens(server, address.port));
      });
    });
  }

  get connectionCount(): number {
    return this.sockets.size;
  }

  private receive(raw: string): void {
    const frame: unknown = JSON.parse(raw);
    if (!validateFrame(frame)) {
      this.invalid.push({ frame, errors: validateFrame.errors });
      return;
    }
    const message = frame as LensMessage;
    this.frames.push(message);
    const index = this.waiters.findIndex((w) => w.type === message.type);
    if (index >= 0) {
      const [waiter] = this.waiters.splice(index, 1);
      waiter.resolve(message);
      return;
    }
    const queue = this.queues.get(message.type) ?? [];
    queue.push(message);
    this.queues.set(message.type, queue);
  }

  /** Next frame of the given type, consuming queued ones first. */
  next<T extends LensMessage["type"]>(
    type: T,
    timeoutMs = 3000,
  ): Promise<Extract<LensMessage, { type: T }>> {
    const queue = this.queues.get(type);
    if (queue && queue.length > 0) {
      return Promise.resolve(queue.shift() as Extract<LensMessage, { type: T }>);
    }
    return new Promise((resolvePromise, reject) => {
      const timer = setTimeout(() => {
        const index = this.waiters.findIndex((w) => w.resolve === done);
        if (index >= 0) this.waiters.splice(index, 1);
        reject(new Error(`timed out waiting for ${type}`));
      }, timeoutMs);
      const done = (m: LensMessage) => {
        clearTimeout(timer);
        resolvePromise(m as Extract<LensMessage, { type: T }>);
      };
      this.waiters.push({ type, resolve: done });
    });
  }

  /** Drops queued frames of a type so a later `next` sees only new ones. */
  drain(type: LensMessage["type"]): LensMessage[] {
    const queue = this.queues.get(type) ?? [];
    this.queues.set(type, []);
    return queue;
  }

  waitForConnection(timeoutMs = 3000): Promise<void> {
    if (this.sockets.size > 0) return Promise.resolve();
    return new Promise((resolvePromise, reject) => {
      const timer = setTimeout(() => reject(new Error("no connection")), timeoutMs);
      this.connections.push(() => {
        clearTimeout(timer);
        resolvePromise();
      });
    });
  }

  send(message: LensMessage): void {
    const raw = JSON.stringify(message);
    for (const socket of this.sockets) socket.send(raw);
  }

  sendRaw(raw: string): void {
    for (const socket of this.sockets) socket.send(raw);
  }

  /** Completes the handshake: waits for hello and answers with welcome. */
  async welcome(): Promise<Extract<LensMessage, { type: "hello" }>> {
    const hello = await this.next("hello");
    this.send({
      v: 1,
      type: "welcome",
      payload: { lensVersion: "0.0.0-test", protocolVersion: 1, sessionId: "s1" },
    });
    return hello;
  }

  /**
   * Waits until every frame the device sent so far has arrived: frames are
   * ordered on the socket, so a pong answers after everything sent before it.
   */
  async sync(): Promise<void> {
    const id = `sync-${Date.now()}-${Math.random()}`;
    this.send({ v: 1, type: "ping", id, payload: {} });
    for (;;) {
      const pong = await this.next("pong");
      if (pong.id === id) return;
    }
  }

  closeClients(code = 1012): void {
    for (const socket of this.sockets) socket.close(code, "test");
  }

  close(): Promise<void> {
    for (const socket of this.sockets) socket.terminate();
    return new Promise((resolvePromise) => this.server.close(() => resolvePromise()));
  }
}

/** Waits until `predicate` holds or the timeout elapses. */
export async function waitFor(predicate: () => boolean, timeoutMs = 3000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!predicate()) {
    if (Date.now() > deadline) throw new Error("waitFor timed out");
    await new Promise((r) => setTimeout(r, 10));
  }
}
