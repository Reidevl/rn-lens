interface WailsWindow {
  _wails?: { environment?: { OS?: string } };
}

/**
 * True in the macOS desktop app, where the window has no title bar and the
 * close, minimize and zoom buttons sit over the top-left of the page.
 * Only the desktop webview's startup script sets `_wails.environment`; in
 * browser mode the Wails runtime creates `_wails` without it.
 */
export function hasInsetTitleBar(win: WailsWindow = window as unknown as WailsWindow): boolean {
  return win._wails?.environment?.OS === "darwin";
}
