'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState } from 'react';
import { Toaster } from 'sonner';
import { AuthGuard } from '@/components/auth/AuthGuard';
import { ErrorBoundary } from '@/components/error/ErrorBoundary';

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 60 * 1000,
        refetchOnWindowFocus: false,
        retry: 1,
      },
    },
  }));

  // AuthGuard wraps every page so protected routes can't flash their content
  // before the session check resolves. When auth is disabled (auth.enabled
  // false), AuthGuard is a no-op pass-through.
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <AuthGuard>{children}</AuthGuard>
        <Toaster theme="dark" position="bottom-right" richColors closeButton />
      </QueryClientProvider>
    </ErrorBoundary>
  );
}
