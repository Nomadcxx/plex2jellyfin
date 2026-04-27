'use client';

import { AlertTriangle, CheckCircle2 } from 'lucide-react';
import { SettingsSaveResponse } from '@/lib/api/client';

type Props = {
  result?: SettingsSaveResponse;
};

export function SubsystemReloadStatus({ result }: Props) {
  if (!result) return null;

  if (result.reload.ok) {
    return (
      <div className="flex items-center gap-2 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-300">
        <CheckCircle2 className="h-4 w-4" />
        <span>Reloaded {result.reload.reloaded?.join(', ') || 'daemon configuration'}</span>
      </div>
    );
  }

  return (
    <div className="rounded-md border border-rose-500/20 bg-rose-500/10 px-3 py-2 text-sm text-rose-200">
      <div className="flex items-center gap-2">
        <AlertTriangle className="h-4 w-4" />
        <span>Reload failed; previous config was restored.</span>
      </div>
      {result.reload.failed?.map((failure) => (
        <div key={`${failure.name}-${failure.error}`} className="mt-1 text-xs text-rose-200/80">
          {failure.name}: {failure.error}
        </div>
      ))}
    </div>
  );
}

