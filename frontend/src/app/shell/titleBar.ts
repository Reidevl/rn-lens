/**
 * True in the macOS desktop app, where the window has no title bar and the
 * close, minimize and zoom buttons sit over the top-left of the page.
 * Browser mode and the other systems keep a normal title bar.
 */
export function hasInsetTitleBar(
  win: { _wails?: unknown } = window as unknown as { _wails?: unknown },
  platform: string = navigator.userAgent,
): boolean {
  return "_wails" in win && win._wails !== undefined && /Macintosh|Mac OS X/.test(platform);
}
