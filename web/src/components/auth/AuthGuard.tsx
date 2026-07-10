"use client";

import { useEffect } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { useAuth } from '@/hooks/useAuth';
import { useSetupStatus } from '@/hooks/useSetup';
import { LoginForm } from './LoginForm';
import { SetupForm } from './SetupForm';
import { AlertTriangle, Loader2, RefreshCw } from 'lucide-react';

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { auth, isLoading } = useAuth();
	const router = useRouter();
	const pathname = usePathname();
	const route = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;
	const authenticated = Boolean(auth?.enabled && auth.authenticated);
	const setup = useSetupStatus(authenticated);
	const redirectTo = authenticated && setup.data
		? setup.data.required && route !== '/setup'
			? '/setup'
			: !setup.data.required && route === '/setup'
				? '/'
				: null
		: null;

	useEffect(() => {
		if (redirectTo) router.replace(redirectTo);
	}, [redirectTo, router]);

	if (isLoading || (authenticated && setup.isLoading) || redirectTo) {
    return (
			<div className="flex min-h-screen items-center justify-center bg-zinc-950">
				<Loader2 className="h-7 w-7 animate-spin text-amber-400" aria-label="Loading application state" />
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

	if (authenticated && setup.isError) {
		return (
			<div className="flex min-h-screen items-center justify-center bg-zinc-950 p-6 text-zinc-100">
				<div className="w-full max-w-md border border-red-900 bg-zinc-950 p-6">
					<AlertTriangle className="mb-4 h-6 w-6 text-red-400" />
					<h1 className="font-mono text-lg font-semibold">Setup state unavailable</h1>
					<p className="mt-2 text-sm text-zinc-400">The server could not determine whether this installation is configured.</p>
					<button
						type="button"
						onClick={() => setup.refetch()}
						className="mt-5 inline-flex h-9 items-center gap-2 border border-zinc-700 px-3 font-mono text-sm hover:border-amber-500 hover:text-amber-300"
					>
						<RefreshCw className="h-4 w-4" />
						Retry
					</button>
				</div>
			</div>
		);
	}

  return <>{children}</>;
}
