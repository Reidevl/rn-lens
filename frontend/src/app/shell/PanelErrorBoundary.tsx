import { Component, type ErrorInfo, type ReactNode } from "react";

import { Button } from "@/shared/ui/atoms/Button";

interface Props {
  /** Panel name shown in the message. */
  panel: string;
  children: ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * Keeps one panel's render error inside that panel: the rail, the status bar
 * and the other panels stay usable, and the message says what broke.
 */
export class PanelErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error(`Lens: the ${this.props.panel} panel crashed`, error, info.componentStack);
  }

  render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div
        role="alert"
        className="flex h-full flex-col items-center justify-center gap-3 p-6 text-sm"
      >
        <p className="font-medium">The {this.props.panel} panel hit an error.</p>
        <pre className="max-w-full overflow-auto rounded-md border border-border bg-card p-3 font-mono text-xs whitespace-pre-wrap text-destructive select-text">
          {error.message}
        </pre>
        <p className="text-muted-foreground">
          Other panels keep working. Please report this with the message above.
        </p>
        <Button size="sm" variant="primary" onClick={() => this.setState({ error: null })}>
          Try again
        </Button>
      </div>
    );
  }
}
