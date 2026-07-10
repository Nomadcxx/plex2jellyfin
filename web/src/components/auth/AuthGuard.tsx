"use client";

import { useAuth } from '@/hooks/useAuth';
import { LoginForm } from './LoginForm';
import { SetupForm } from './SetupForm';
import { Loader2 } from 'lucide-react';

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { auth, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-zinc-600" />
      </div>
    );
  }

  // First run: no password configured yet — force creating one before the
  // dashboard is reachable.
  if (auth && !auth.enabled) {
    return <SetupForm />;
  }

  if (auth?.enabled && !auth?.authenticated) {
    return <LoginForm />;
  }

  return <>{children}</>;
}
