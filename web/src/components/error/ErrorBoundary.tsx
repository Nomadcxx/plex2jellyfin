"use client";

import { Component, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex items-center justify-center min-h-screen bg-zinc-950 text-zinc-100">
          <div className="p-6 bg-zinc-900 rounded-lg border border-red-500/30 max-w-md">
            <h2 className="text-xl font-bold text-red-400 mb-2">Something went wrong</h2>
            <p className="text-zinc-400 text-sm">{this.state.error?.message}</p>
            <button onClick={() => window.location.reload()} className="mt-4 px-4 py-2 bg-zinc-800 rounded hover:bg-zinc-700">Reload</button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
