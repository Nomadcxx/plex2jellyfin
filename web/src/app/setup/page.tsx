'use client';

import { Loader2 } from 'lucide-react';
import { SetupWizard } from '@/components/setup/SetupWizard';
import { useSetupStatus } from '@/hooks/useSetup';

export default function SetupPage() {
  const setup = useSetupStatus();

  if (!setup.data) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-zinc-950">
        <Loader2 className="h-7 w-7 animate-spin text-amber-400" aria-label="Loading setup" />
      </div>
    );
  }

  return <SetupWizard status={setup.data} />;
}
