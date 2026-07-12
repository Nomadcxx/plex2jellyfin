'use client';

import { useState } from 'react';
import { Loader2, RefreshCw, Wrench } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { ConfirmReversible } from '@/components/settings/ConfirmReversible';
import { checkArrCompatibility, CompatibilityResult } from '@/lib/api/client';
import { useSettingsSection } from '@/hooks/useSettings';

type Props = {
  service: 'sonarr' | 'radarr';
};

export function CompatibilityPanel({ service }: Props) {
  const { data } = useSettingsSection(service);
  const [report, setReport] = useState<CompatibilityResult | null>(null);
  const [busy, setBusy] = useState<'check' | 'fix' | null>(null);
  const [confirmFix, setConfirmFix] = useState(false);

  const url = typeof data?.url === 'string' ? data.url : '';
  const apiKey = typeof data?.api_key === 'string' ? data.api_key : '';
  const enabled = data?.enabled === true;
  const canRun = enabled && !!url && !!apiKey;

  const run = async (fix: boolean) => {
    if (!canRun) return;
    setBusy(fix ? 'fix' : 'check');
    try {
      const result = await checkArrCompatibility(service, { url, api_key: apiKey }, fix);
      setReport(result);
      if (result.error) {
        toast.error(result.error);
      } else if (result.healthy) {
        toast.success(fix ? 'Compatibility settings updated' : 'Compatibility looks good');
      } else if (!fix) {
        toast.message(`${result.issues?.length ?? 0} setting(s) need attention`);
      } else {
        toast.error(result.error || 'Compatibility repair failed');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Compatibility check failed');
    } finally {
      setBusy(null);
      setConfirmFix(false);
    }
  };

  const issues = report?.issues ?? [];

  return (
    <div className="space-y-3 border border-zinc-800 bg-zinc-950/40 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-mono text-sm font-semibold text-zinc-200">Arr compatibility</h2>
          <p className="mt-1 text-sm text-zinc-500">
            Completed Download Handling and rename settings must match plex2jellyfin.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!canRun || busy !== null}
            onClick={() => run(false)}
          >
            {busy === 'check' ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            Check
          </Button>
          {issues.length > 0 && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!canRun || busy !== null}
              onClick={() => setConfirmFix(true)}
            >
              {busy === 'fix' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Wrench className="h-4 w-4" />}
              Fix
            </Button>
          )}
        </div>
      </div>

      {!enabled && (
        <p className="text-sm text-zinc-500">Enable {service} and save credentials before checking.</p>
      )}

      {report?.error && !report.ok && (
        <p className="text-sm text-rose-300">{report.error}</p>
      )}

      {report && report.ok && report.healthy && (
        <p className="text-sm text-emerald-300">Healthy — no compatibility issues.</p>
      )}

      {issues.length > 0 && (
        <ul className="space-y-2 border-l-2 border-amber-500 bg-amber-950/10 px-4 py-3 text-sm">
          {issues.map((issue) => (
            <li key={`${issue.setting}-${issue.current}`}>
              <span className="font-mono text-amber-100">{issue.setting}</span>
              <span className="text-zinc-400">
                {' '}
                is {issue.current || '(empty)'}, expected {issue.expected}
              </span>
            </li>
          ))}
        </ul>
      )}

      <ConfirmReversible
        open={confirmFix}
        title={`Update ${service} compatibility settings`}
        onCancel={() => setConfirmFix(false)}
        onConfirm={() => run(true)}
      >
        This will change Completed Download Handling and rename settings on the remote {service} instance.
      </ConfirmReversible>
    </div>
  );
}
