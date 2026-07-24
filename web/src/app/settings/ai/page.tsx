'use client';

import { useEffect, useState } from 'react';
import { Loader2, Save, Wifi } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Badge } from '@/components/ui/badge';
import {
  useSettingsSection,
  useUpdateSettingsSection,
  useTestAIConnection,
  useAIModels,
} from '@/hooks/useSettings';
import { ModelSelect } from '@/components/settings/ModelSelect';
import { buildAISettingsPayload } from './settingsPayload';

function numberInputValue(value: unknown): string | number {
  return typeof value === 'number' || typeof value === 'string' ? value : '';
}

export default function AISettingsPage() {
  const { data, isLoading } = useSettingsSection('ai');
  const update = useUpdateSettingsSection('ai');
  const testConnection = useTestAIConnection();
  const [draft, setDraft] = useState<Record<string, unknown>>({});
  const endpoint = typeof draft.ollama_endpoint === 'string' ? draft.ollama_endpoint : '';
  const modelsQuery = useAIModels(endpoint || undefined, true);
  const models = modelsQuery.data?.models ?? [];
  const modelsErr = !modelsQuery.data?.success ? modelsQuery.data?.message ?? null : null;

  useEffect(() => {
    if (data) setDraft(data);
  }, [data]);

  const set = (key: string, value: unknown) =>
    setDraft((prev) => ({ ...prev, [key]: value }));

  const submit = () => {
    update.mutate(buildAISettingsPayload(draft), {
      onSuccess: (result) => {
        if (result.reload.ok) toast.success('AI settings saved');
        else toast.error('Daemon reload failed; previous config restored');
      },
      onError: () => toast.error('Failed to save AI settings'),
    });
  };

  const runConnectionTest = () => {
    testConnection.mutate(
      { endpoint: String(draft.ollama_endpoint ?? '') },
      {
        onSuccess: (result) => {
          if (result.success)
            toast.success(
              result.modelCount != null
                ? `Connected — ${result.modelCount} model(s) available (${result.latencyMs}ms)`
                : 'Connected',
            );
          else toast.error(result.message ?? 'Connection failed');
        },
        onError: () => toast.error('Connection test failed'),
      },
    );
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">AI</h1>
        <p className="mt-1 text-sm text-zinc-400">Ollama model and AI enhancement settings.</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>AI</CardTitle>
          <CardDescription>Ollama model and AI enhancement settings.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {isLoading ? (
            <div className="flex items-center gap-2 text-sm text-zinc-400">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading settings
            </div>
          ) : (
            <div className="grid gap-5 md:grid-cols-2">
              {/* Enabled toggle — full width */}
              <label className="col-span-full flex items-center justify-between rounded-lg border border-zinc-800 px-4 py-3">
                <div>
                  <p className="text-sm font-medium text-zinc-300">Enabled</p>
                  <p className="text-xs text-zinc-500">Enable AI-assisted title matching</p>
                </div>
                <Switch
                  checked={Boolean(draft.enabled)}
                  onCheckedChange={(v) => set('enabled', v)}
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Ollama endpoint</span>
                <Input
                  value={typeof draft.ollama_endpoint === 'string' ? draft.ollama_endpoint : ''}
                  onChange={(e) => set('ollama_endpoint', e.target.value)}
                  placeholder="http://localhost:11434"
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Primary model</span>
                <ModelSelect
                  value={typeof draft.model === 'string' ? draft.model : ''}
                  onChange={(v) => set('model', v)}
                  models={models}
                  loading={modelsQuery.isFetching}
                  error={modelsErr}
                  onRefresh={() => modelsQuery.refetch()}
                  placeholder="llama3"
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Fallback model</span>
                <ModelSelect
                  value={typeof draft.fallback_model === 'string' ? draft.fallback_model : ''}
                  onChange={(v) => set('fallback_model', v)}
                  models={models}
                  loading={modelsQuery.isFetching}
                  error={modelsErr}
                  onRefresh={() => modelsQuery.refetch()}
                  placeholder="mistral"
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Cloud model</span>
                <Input
                  value={typeof draft.cloud_model === 'string' ? draft.cloud_model : ''}
                  onChange={(e) => set('cloud_model', e.target.value)}
                  placeholder="gpt-4o-mini"
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Confidence threshold</span>
                <Input
                  type="number"
                  min={0}
                  max={1}
                  step={0.01}
                  value={numberInputValue(draft.confidence_threshold)}
                  onChange={(e) => set('confidence_threshold', e.target.value)}
                  placeholder="0.85"
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Auto-trigger threshold</span>
                <Input
                  type="number"
                  min={0}
                  max={1}
                  step={0.01}
                  value={numberInputValue(draft.auto_trigger_threshold)}
                  onChange={(e) => set('auto_trigger_threshold', e.target.value)}
                  placeholder="0.65"
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Timeout (seconds)</span>
                <Input
                  type="number"
                  min={1}
                  max={600}
                  value={numberInputValue(draft.timeout_seconds)}
                  onChange={(e) => set('timeout_seconds', e.target.value)}
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Max retries</span>
                <Input
                  type="number"
                  min={0}
                  max={20}
                  value={numberInputValue(draft.max_retries)}
                  onChange={(e) => set('max_retries', e.target.value)}
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Hourly limit</span>
                <Input
                  type="number"
                  min={0}
                  value={numberInputValue(draft.hourly_limit)}
                  onChange={(e) => set('hourly_limit', e.target.value)}
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Daily limit</span>
                <Input
                  type="number"
                  min={0}
                  value={numberInputValue(draft.daily_limit)}
                  onChange={(e) => set('daily_limit', e.target.value)}
                />
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">Enhancement interval (seconds)</span>
                <Input
                  type="number"
                  min={1}
                  value={numberInputValue(draft.enhancement_interval_seconds)}
                  onChange={(e) => set('enhancement_interval_seconds', e.target.value)}
                />
              </label>

              <label className="flex items-center justify-between rounded-lg border border-zinc-800 px-4 py-3 text-sm">
                <div>
                  <p className="font-medium text-zinc-300">Cache enabled</p>
                  <p className="text-xs text-zinc-500">Cache AI responses to reduce API calls</p>
                </div>
                <Switch
                  checked={Boolean(draft.cache_enabled)}
                  onCheckedChange={(v) => set('cache_enabled', v)}
                />
              </label>

              <label className="flex items-center justify-between rounded-lg border border-zinc-800 px-4 py-3 text-sm">
                <div>
                  <p className="font-medium text-zinc-300">Auto-resolve risky matches</p>
                  <p className="text-xs text-zinc-500">Automatically accept low-confidence suggestions</p>
                </div>
                <Switch
                  checked={Boolean(draft.auto_resolve_risky)}
                  onCheckedChange={(v) => set('auto_resolve_risky', v)}
                />
              </label>
            </div>
          )}

          {testConnection.data && (
            <div className="flex items-center gap-2 text-sm">
              <Badge
                variant="outline"
                className={
                  testConnection.data.success
                    ? 'border-green-500/40 text-green-400'
                    : 'border-red-500/40 text-red-400'
                }
              >
                {testConnection.data.success ? 'Online' : 'Offline'}
              </Badge>
              {testConnection.data.message && (
                <span className="text-zinc-400">{testConnection.data.message}</span>
              )}
            </div>
          )}

          <div className="flex flex-wrap gap-3">
            <Button
              variant="outline"
              onClick={runConnectionTest}
              disabled={testConnection.isPending}
            >
              {testConnection.isPending ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Wifi className="mr-2 h-4 w-4" />
              )}
              Test connection
            </Button>
            <Button onClick={submit} disabled={update.isPending || isLoading}>
              {update.isPending ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Save className="mr-2 h-4 w-4" />
              )}
              Save
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
