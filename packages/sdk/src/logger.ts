export interface LensLogger {
  debug?(message: string, ...rest: unknown[]): void;
  info?(message: string, ...rest: unknown[]): void;
  warn?(message: string, ...rest: unknown[]): void;
  error?(message: string, ...rest: unknown[]): void;
}

export interface Log {
  debug(message: string, ...rest: unknown[]): void;
  info(message: string, ...rest: unknown[]): void;
  warn(message: string, ...rest: unknown[]): void;
  error(message: string, ...rest: unknown[]): void;
}

const noop = (): void => {};

/** Wraps a partial logger so every level is safe to call. */
export function makeLog(logger: LensLogger | undefined): Log {
  const bind = (level: keyof LensLogger) => {
    const fn = logger?.[level];
    if (typeof fn !== "function") return noop;
    return (message: string, ...rest: unknown[]) => fn.call(logger, `[lens] ${message}`, ...rest);
  };
  return { debug: bind("debug"), info: bind("info"), warn: bind("warn"), error: bind("error") };
}
