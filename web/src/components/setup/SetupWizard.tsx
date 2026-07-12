'use client';

import Image from 'next/image';
import { useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  Circle,
  Cpu,
  Film,
  FolderInput,
  FolderOutput,
  Gauge,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  Server,
  Trash2,
  Tv,
  Wrench,
} from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { ModelSelect } from '@/components/settings/ModelSelect';
import { SecretField } from '@/components/settings/SecretField';
import { ConfirmReversible } from '@/components/settings/ConfirmReversible';
import {
  checkArrCompatibility,
  CompatibilityResult,
  preflightPath,
  testSettingsConnection,
} from '@/lib/api/client';
import { useAIModels, useTestAIConnection, useTestAIPrompt } from '@/hooks/useSettings';
import { SetupDraft, SetupStatus, setupKeys, useApplySetup } from '@/hooks/useSetup';
import {
  emptyChecks,
  hydrateDraft,
  pathCheckKey,
  RuntimeKind,
  ServiceName,
  serviceFingerprint,
  SetupStep,
  stepErrors,
  WizardChecks,
} from './setupDraft';

const steps: Array<{ id: SetupStep; label: string; icon: typeof Tv }> = [
  { id: 'media', label: 'Media', icon: Tv },
  { id: 'services', label: 'Services', icon: Server },
  { id: 'ai', label: 'AI', icon: Cpu },
  { id: 'runtime', label: 'Runtime', icon: Gauge },
  { id: 'review', label: 'Review', icon: CheckCircle2 },
];

export function SetupWizard({ status }: { status: SetupStatus }) {
  const runtime = status.runtime.kind as RuntimeKind;
  const [draft, setDraft] = useState(() => {
    const next = hydrateDraft(status.draft, runtime);
    if (next.jellyfin.enabled && !next.jellyfin.plugin_daemon_url && status.default_callback_url) {
      next.jellyfin.plugin_daemon_url = status.default_callback_url;
    }
    return next;
  });
  const [checks, setChecks] = useState<WizardChecks>(() => emptyChecks());
  const [stepIndex, setStepIndex] = useState(0);
  const [visibleErrors, setVisibleErrors] = useState<string[]>([]);
  const [compatibility, setCompatibility] = useState<Partial<Record<'sonarr' | 'radarr', CompatibilityResult>>>({});
  const [testingService, setTestingService] = useState<ServiceName | null>(null);
  const [fixTarget, setFixTarget] = useState<'sonarr' | 'radarr' | null>(null);
  const [applied, setApplied] = useState(false);
  const [scanWarning, setScanWarning] = useState('');
  const apply = useApplySetup();
  const router = useRouter();
  const queryClient = useQueryClient();
  const current = steps[stepIndex];

  const setPath = (collection: 'watch' | 'libraries', media: 'tv' | 'movies', paths: string[]) => {
    setDraft((value) => ({ ...value, [collection]: { ...value[collection], [media]: paths } }));
  };

  const next = () => {
    const errors = stepErrors(current.id, draft, checks, runtime);
    if (errors.length > 0) {
      setVisibleErrors(errors);
      return;
    }
    setVisibleErrors([]);
    setStepIndex((index) => Math.min(index + 1, steps.length - 1));
  };

  const finish = () => {
    queryClient.setQueryData(setupKeys.status, { ...status, required: false, complete: true, daemon_state: 'running' });
    router.replace('/');
  };

  const runServiceTest = async (name: ServiceName) => {
    const value = draft[name];
    setTestingService(name);
    try {
      const result = await testSettingsConnection(name, value);
      if (!result.ok) throw new Error(result.error || `${name} connection failed`);
      setChecks((state) => ({
        ...state,
        services: { ...state.services, [name]: serviceFingerprint(value) },
      }));
      if (name === 'sonarr' || name === 'radarr') {
        const report = await checkArrCompatibility(name, value);
        setCompatibility((state) => ({ ...state, [name]: report }));
      }
      toast.success(`${label(name)} connected${result.version ? ` (${result.version})` : ''}`);
    } catch (error) {
      setChecks((state) => ({ ...state, services: { ...state.services, [name]: undefined } }));
      toast.error(error instanceof Error ? error.message : `${label(name)} connection failed`);
    } finally {
      setTestingService(null);
    }
  };

  const fixCompatibility = async () => {
    if (!fixTarget) return;
    const result = await checkArrCompatibility(fixTarget, draft[fixTarget], true);
    setCompatibility((state) => ({ ...state, [fixTarget]: result }));
    setFixTarget(null);
    result.healthy ? toast.success(`${label(fixTarget)} settings updated`) : toast.error(result.error || 'Compatibility repair failed');
  };

  if (applied) {
    return <SetupComplete scanWarning={scanWarning} onDashboard={finish} />;
  }

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <div className="mx-auto flex min-h-screen w-full max-w-[1440px] flex-col lg:flex-row">
        <aside className="border-b border-zinc-800 px-5 py-4 lg:w-60 lg:shrink-0 lg:border-b-0 lg:border-r lg:px-6 lg:py-8">
          <div className="flex items-center justify-between gap-4 lg:block">
            <div>
              <Image src="/p2j-mark.png" alt="P2J" width={150} height={58} priority className="h-auto w-[120px]" />
              <p className="mt-2 font-mono text-xs uppercase text-zinc-500">First-run setup</p>
            </div>
            <div className="font-mono text-xs text-zinc-500 lg:mt-12">{stepIndex + 1}/{steps.length}</div>
          </div>
          <nav aria-label="Setup progress" className="mt-4 hidden space-y-1 lg:block">
            {steps.map((step, index) => {
              const Icon = step.icon;
              const active = index === stepIndex;
              const complete = index < stepIndex;
              return (
                <button
                  key={step.id}
                  type="button"
                  disabled={index > stepIndex}
                  onClick={() => { setStepIndex(index); setVisibleErrors([]); }}
                  className={`flex h-10 w-full items-center gap-3 border-l-2 px-3 text-left font-mono text-sm ${active ? 'border-amber-400 bg-zinc-900 text-amber-300' : complete ? 'border-green-700 text-zinc-300 hover:bg-zinc-900' : 'border-zinc-800 text-zinc-600'}`}
                >
                  {complete ? <Check className="h-4 w-4 text-green-400" /> : <Icon className="h-4 w-4" />}
                  {step.label}
                </button>
              );
            })}
          </nav>
          <div className="mt-3 h-1 overflow-hidden bg-zinc-900 lg:hidden">
            <div className="h-full bg-amber-400 transition-all" style={{ width: `${((stepIndex + 1) / steps.length) * 100}%` }} />
          </div>
        </aside>

        <main className="flex min-w-0 flex-1 flex-col">
          <header className="border-b border-zinc-800 px-5 py-5 sm:px-8 lg:px-10">
            <p className="font-mono text-xs uppercase text-amber-400">setup::{current.id}</p>
            <h1 className="mt-1 text-2xl font-semibold">{stepTitle(current.id)}</h1>
          </header>
          <div className="flex-1 px-5 py-6 sm:px-8 lg:px-10">
            <div className="w-full max-w-5xl">
              {current.id === 'media' && <MediaStep draft={draft} checks={checks} setChecks={setChecks} setPath={setPath} />}
              {current.id === 'services' && (
                <ServicesStep
                  draft={draft}
                  setDraft={setDraft}
                  checks={checks}
                  compatibility={compatibility}
                  testing={testingService}
                  onTest={runServiceTest}
                  onFix={setFixTarget}
                  defaultCallbackURL={status.default_callback_url ?? ''}
                />
              )}
              {current.id === 'ai' && <AIStep draft={draft} setDraft={setDraft} checks={checks} setChecks={setChecks} />}
              {current.id === 'runtime' && <RuntimeStep draft={draft} setDraft={setDraft} setChecks={setChecks} runtime={runtime} uid={status.runtime.uid} gid={status.runtime.gid} />}
              {current.id === 'review' && <ReviewStep draft={draft} runtime={runtime} />}

              {visibleErrors.length > 0 && (
                <div role="alert" className="mt-6 border border-red-900 bg-red-950/20 p-4">
                  <div className="flex items-center gap-2 font-mono text-sm text-red-300"><AlertTriangle className="h-4 w-4" />Resolve before continuing</div>
                  <ul className="mt-2 space-y-1 text-sm text-red-200/80">
                    {visibleErrors.map((error) => <li key={error}>{error}</li>)}
                  </ul>
                </div>
              )}

              {apply.isError && (
                <div role="alert" className="mt-6 border border-red-900 bg-red-950/20 p-4 text-sm text-red-200">
                  {apply.error instanceof Error ? apply.error.message : 'Setup could not be applied.'}
                </div>
              )}
            </div>
          </div>
          <footer className="flex min-h-16 items-center justify-between border-t border-zinc-800 px-5 py-3 sm:px-8 lg:px-10">
            <Button variant="outline" onClick={() => { setStepIndex((index) => Math.max(0, index - 1)); setVisibleErrors([]); }} disabled={stepIndex === 0 || apply.isPending}>
              <ArrowLeft className="h-4 w-4" />Back
            </Button>
            {current.id === 'review' ? (
              <Button
                onClick={() => {
                  const errors = stepErrors('review', draft, checks, runtime);
                  if (errors.length) return setVisibleErrors(errors);
                  apply.mutate(draft, {
                    onSuccess: (result) => {
                      setApplied(true);
                      if (result.plugin_warning) {
                        toast.warning(result.plugin_warning);
                      }
                      if (result.scan_warning) {
                        setScanWarning(result.scan_warning);
                        toast.warning(result.scan_warning);
                      }
                    },
                  });
                }}
                disabled={apply.isPending}
              >
                {apply.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                {apply.isPending ? 'Applying and indexing…' : 'Apply and start'}
              </Button>
            ) : (
              <Button onClick={next}>Continue<ArrowRight className="h-4 w-4" /></Button>
            )}
          </footer>
        </main>
      </div>

      <ConfirmReversible open={fixTarget !== null} title={`Update ${fixTarget ? label(fixTarget) : ''} settings`} onCancel={() => setFixTarget(null)} onConfirm={fixCompatibility}>
        Plex2Jellyfin will disable completed download handling and enable media renaming where required.
      </ConfirmReversible>
    </div>
  );
}

function MediaStep({ draft, checks, setChecks, setPath }: {
  draft: SetupDraft;
  checks: WizardChecks;
  setChecks: React.Dispatch<React.SetStateAction<WizardChecks>>;
  setPath: (collection: 'watch' | 'libraries', media: 'tv' | 'movies', paths: string[]) => void;
}) {
  return (
    <div className="space-y-8">
      {([
        ['tv', 'TV', Tv],
        ['movies', 'Movies', Film],
      ] as const).map(([media, title, Icon]) => (
        <section key={media} aria-labelledby={`${media}-paths`} className="border-t border-zinc-800 pt-5 first:border-t-0 first:pt-0">
          <div className="mb-4 flex items-center gap-2"><Icon className="h-5 w-5 text-amber-400" /><h2 id={`${media}-paths`} className="font-mono text-base font-semibold">{title}</h2><span className="text-xs text-zinc-600">optional</span></div>
          <div className="grid gap-5 xl:grid-cols-2">
            <PathEditor title="Incoming" icon={FolderInput} collection="watch" media={media} paths={draft.watch[media]} writable={draft.runtime.delete_source} checks={checks} setChecks={setChecks} onChange={(paths) => setPath('watch', media, paths)} />
            <PathEditor title="Library" icon={FolderOutput} collection="libraries" media={media} paths={draft.libraries[media]} writable checks={checks} setChecks={setChecks} onChange={(paths) => setPath('libraries', media, paths)} />
          </div>
        </section>
      ))}
    </div>
  );
}

function PathEditor({ title, icon: Icon, collection, media, paths, writable, checks, setChecks, onChange }: {
  title: string;
  icon: typeof FolderInput;
  collection: 'watch' | 'libraries';
  media: 'tv' | 'movies';
  paths: string[];
  writable: boolean;
  checks: WizardChecks;
  setChecks: React.Dispatch<React.SetStateAction<WizardChecks>>;
  onChange: (paths: string[]) => void;
}) {
  const [value, setValue] = useState('');
  const [checking, setChecking] = useState<string | null>(null);

  const verify = async (path: string) => {
    setChecking(path);
    const result = await preflightPath(path, collection === 'watch' ? 'watch' : 'library');
    const ok = result.exists && result.is_dir && result.readable && (!writable || result.writable);
    setChecks((state) => ({ ...state, paths: { ...state.paths, [pathCheckKey(collection, media, path)]: ok } }));
    setChecking(null);
    ok ? toast.success(`${path} verified`) : toast.error(result.warnings?.join('; ') || `${path} is not accessible`);
    return ok;
  };

  return (
    <fieldset className="min-w-0 border border-zinc-800 p-4">
      <legend className="px-2 font-mono text-sm text-zinc-300"><span className="inline-flex items-center gap-2"><Icon className="h-4 w-4" />{title}</span></legend>
      <div className="flex gap-2">
        <Input value={value} onChange={(event) => setValue(event.target.value)} placeholder={`/${collection === 'watch' ? 'watch' : 'library'}/${media}`} />
        <Button type="button" size="icon" aria-label={`Add ${title.toLowerCase()} path`} disabled={!value.trim() || checking !== null} onClick={async () => { const path = value.trim(); if (await verify(path)) { onChange([...paths, path]); setValue(''); } }}>
          {checking === value.trim() ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
        </Button>
      </div>
      <div className="mt-3 min-h-10 space-y-2">
        {paths.length === 0 ? <p className="py-2 text-sm text-zinc-600">Not configured</p> : paths.map((path, index) => {
          const state = checks.paths[pathCheckKey(collection, media, path)];
          return (
            <div key={`${path}-${index}`} className="flex min-h-10 items-center gap-2 border-t border-zinc-900 pt-2">
              {state === true ? <CheckCircle2 className="h-4 w-4 shrink-0 text-green-400" /> : state === false ? <AlertTriangle className="h-4 w-4 shrink-0 text-red-400" /> : <Circle className="h-4 w-4 shrink-0 text-zinc-600" />}
              <span className="min-w-0 flex-1 truncate font-mono text-xs text-zinc-300" title={path}>{path}</span>
              <Button type="button" variant="ghost" size="icon" aria-label={`Verify ${path}`} title="Verify path" disabled={checking !== null} onClick={() => verify(path)}>{checking === path ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}</Button>
              <Button type="button" variant="ghost" size="icon" aria-label={`Remove ${path}`} title="Remove path" onClick={() => onChange(paths.filter((_, item) => item !== index))}><Trash2 className="h-4 w-4" /></Button>
            </div>
          );
        })}
      </div>
    </fieldset>
  );
}

function ServicesStep({ draft, setDraft, checks, compatibility, testing, onTest, onFix, defaultCallbackURL }: {
  draft: SetupDraft;
  setDraft: React.Dispatch<React.SetStateAction<SetupDraft>>;
  checks: WizardChecks;
  compatibility: Partial<Record<'sonarr' | 'radarr', CompatibilityResult>>;
  testing: ServiceName | null;
  onTest: (name: ServiceName) => void;
  onFix: (name: 'sonarr' | 'radarr') => void;
  defaultCallbackURL: string;
}) {
  const update = (name: ServiceName, patch: Record<string, unknown>) => setDraft((value) => ({ ...value, [name]: { ...value[name], ...patch } }));
  return (
    <div className="divide-y divide-zinc-800 border-y border-zinc-800">
      {(['sonarr', 'radarr', 'jellyfin'] as ServiceName[]).map((name) => {
        const value = draft[name];
        const verified = checks.services[name] === serviceFingerprint(value);
        const report = name === 'jellyfin' ? undefined : compatibility[name];
        return (
          <section key={name} className="py-6">
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-3"><Server className="h-5 w-5 text-amber-400" /><h2 className="font-mono font-semibold">{label(name)}</h2>{verified && <span className="font-mono text-xs text-green-400">verified</span>}</div>
              <Switch checked={value.enabled} onCheckedChange={(enabled) => update(name, { enabled })} aria-label={`Enable ${label(name)}`} />
            </div>
            {value.enabled && (
              <div className="mt-5 grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] md:items-end">
                <label className="space-y-2 text-sm"><span className="text-zinc-400">URL</span><Input value={value.url} onChange={(event) => update(name, { url: event.target.value })} placeholder={serviceURL(name)} /></label>
                <label className="space-y-2 text-sm"><span className="text-zinc-400">API key</span><SecretField value={value.api_key} onChange={(api_key) => update(name, { api_key })} /></label>
                <Button variant="outline" disabled={testing !== null || !value.url || !value.api_key} onClick={() => onTest(name)}>{testing === name ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}Test</Button>
              </div>
            )}
            {report && (report.issues?.length ?? 0) > 0 && (
              <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-l-2 border-amber-500 bg-amber-950/10 px-4 py-3 text-sm">
                <span className="text-amber-200">{report.issues.length} compatibility setting{report.issues.length === 1 ? '' : 's'} need attention.</span>
                <Button size="sm" variant="outline" onClick={() => onFix(name as 'sonarr' | 'radarr')}><Wrench className="h-4 w-4" />Review fix</Button>
              </div>
            )}
            {name === 'jellyfin' && value.enabled && (
              <>
                <div className="mt-5 space-y-4 border-t border-zinc-900 pt-4">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <h3 className="font-mono text-sm text-zinc-300">Install companion plugin</h3>
                      <p className="mt-1 text-sm text-zinc-500">Closes the feedback loop with Jellyfin library events.</p>
                    </div>
                    <Switch
                      checked={!!draft.jellyfin.plugin_install}
                      onCheckedChange={(plugin_install) => {
                        const patch: Record<string, unknown> = { plugin_install };
                        if (plugin_install && !draft.jellyfin.plugin_daemon_url && defaultCallbackURL) {
                          patch.plugin_daemon_url = defaultCallbackURL;
                        }
                        update('jellyfin', patch);
                      }}
                      aria-label="Install companion plugin"
                    />
                  </div>
                  {draft.jellyfin.plugin_install && (
                    <>
                      <div className="flex items-center justify-between gap-4">
                        <div>
                          <h3 className="font-mono text-sm text-zinc-300">Restart Jellyfin after install</h3>
                          <p className="mt-1 text-sm text-zinc-500">Required for the plugin to load; otherwise verify later from the CLI.</p>
                        </div>
                        <Switch
                          checked={!!draft.jellyfin.plugin_restart}
                          onCheckedChange={(plugin_restart) => update('jellyfin', { plugin_restart })}
                          aria-label="Restart Jellyfin after plugin install"
                        />
                      </div>
                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-400">Plugin callback URL</span>
                        <Input
                          value={draft.jellyfin.plugin_daemon_url ?? ''}
                          onChange={(event) => update('jellyfin', { plugin_daemon_url: event.target.value })}
                          placeholder={defaultCallbackURL || 'http://192.168.1.10:5522'}
                        />
                        <span className="block text-xs text-zinc-500">From Jellyfin&apos;s point of view — never localhost when Jellyfin is in a container.</span>
                      </label>
                    </>
                  )}
                </div>
                <PathMappings draft={draft} setDraft={setDraft} />
              </>
            )}
          </section>
        );
      })}
    </div>
  );
}

function PathMappings({ draft, setDraft }: { draft: SetupDraft; setDraft: React.Dispatch<React.SetStateAction<SetupDraft>> }) {
  const mappings = draft.jellyfin.path_mappings;
  const update = (next: typeof mappings) => setDraft((value) => ({ ...value, jellyfin: { ...value.jellyfin, path_mappings: next } }));
  return (
    <div className="mt-5 border-t border-zinc-900 pt-4">
      <div className="flex items-center justify-between"><h3 className="font-mono text-sm text-zinc-300">Path mappings</h3><Button type="button" size="sm" variant="ghost" onClick={() => update([...mappings, { jellyfin: '', daemon: '' }])}><Plus className="h-4 w-4" />Add mapping</Button></div>
      <div className="mt-3 space-y-2">
        {mappings.map((mapping, index) => (
          <div key={index} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
            <Input value={mapping.jellyfin} onChange={(event) => update(mappings.map((item, i) => i === index ? { ...item, jellyfin: event.target.value } : item))} placeholder="Jellyfin path" aria-label={`Jellyfin path mapping ${index + 1}`} />
            <Input value={mapping.daemon} onChange={(event) => update(mappings.map((item, i) => i === index ? { ...item, daemon: event.target.value } : item))} placeholder="P2J path" aria-label={`P2J path mapping ${index + 1}`} />
            <Button type="button" variant="ghost" size="icon" onClick={() => update(mappings.filter((_, i) => i !== index))} aria-label={`Remove path mapping ${index + 1}`}><Trash2 className="h-4 w-4" /></Button>
          </div>
        ))}
      </div>
    </div>
  );
}

function AIStep({ draft, setDraft, checks, setChecks }: {
  draft: SetupDraft;
  setDraft: React.Dispatch<React.SetStateAction<SetupDraft>>;
  checks: WizardChecks;
  setChecks: React.Dispatch<React.SetStateAction<WizardChecks>>;
}) {
  const testConnection = useTestAIConnection();
  const testPrompt = useTestAIPrompt();
  const connected = checks.ai === draft.ai.endpoint.trim();
  const models = useAIModels(draft.ai.endpoint, draft.ai.enabled && connected);
  const update = (patch: Partial<SetupDraft['ai']>) => setDraft((value) => ({ ...value, ai: { ...value.ai, ...patch } }));

  const connect = () => testConnection.mutate({ endpoint: draft.ai.endpoint }, {
    onSuccess: (result) => {
      if (result.success) {
        setChecks((state) => ({ ...state, ai: draft.ai.endpoint.trim() }));
        toast.success(result.message || 'Ollama connected');
      } else toast.error(result.message || 'Ollama connection failed');
    },
  });

  return (
    <div className="space-y-6 border-y border-zinc-800 py-6">
      <div className="flex items-center justify-between gap-4"><div className="flex items-center gap-3"><Cpu className="h-5 w-5 text-amber-400" /><h2 className="font-mono font-semibold">Ollama</h2>{connected && <span className="font-mono text-xs text-green-400">verified</span>}</div><Switch checked={draft.ai.enabled} onCheckedChange={(enabled) => update({ enabled })} aria-label="Enable AI matching" /></div>
      {draft.ai.enabled && (
        <>
          <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
            <label className="space-y-2 text-sm"><span className="text-zinc-400">Endpoint</span><Input value={draft.ai.endpoint} onChange={(event) => update({ endpoint: event.target.value })} placeholder="http://localhost:11434" /></label>
            <Button variant="outline" onClick={connect} disabled={testConnection.isPending || !draft.ai.endpoint}>{testConnection.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}Test</Button>
          </div>
          <div className="grid gap-5 md:grid-cols-2">
            <label className="space-y-2 text-sm"><span className="text-zinc-400">Primary model</span><ModelSelect value={draft.ai.primary_model} onChange={(primary_model) => update({ primary_model })} models={models.data?.models ?? []} loading={models.isFetching} error={models.data?.success === false ? models.data.message ?? 'Model discovery failed' : null} onRefresh={() => models.refetch()} /></label>
            <label className="space-y-2 text-sm"><span className="text-zinc-400">Fallback model</span><ModelSelect value={draft.ai.fallback_model} onChange={(fallback_model) => update({ fallback_model })} models={models.data?.models ?? []} loading={models.isFetching} error={null} onRefresh={() => models.refetch()} placeholder="Optional" /></label>
          </div>
          <div className="flex items-center gap-3">
            <Button variant="outline" disabled={!connected || !draft.ai.primary_model || testPrompt.isPending} onClick={() => testPrompt.mutate({ endpoint: draft.ai.endpoint, model: draft.ai.primary_model })}>{testPrompt.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Cpu className="h-4 w-4" />}Run prompt test</Button>
            {testPrompt.data && <span className={`text-sm ${testPrompt.data.success ? 'text-green-400' : 'text-red-400'}`}>{testPrompt.data.success ? `${testPrompt.data.durationMs ?? 0}ms` : 'Prompt failed'}</span>}
          </div>
        </>
      )}
    </div>
  );
}

function RuntimeStep({ draft, setDraft, setChecks, runtime, uid, gid }: {
  draft: SetupDraft;
  setDraft: React.Dispatch<React.SetStateAction<SetupDraft>>;
  setChecks: React.Dispatch<React.SetStateAction<WizardChecks>>;
  runtime: RuntimeKind;
  uid: number;
  gid: number;
}) {
  const updateRuntime = (patch: Partial<SetupDraft['runtime']>) => setDraft((value) => ({ ...value, runtime: { ...value.runtime, ...patch } }));
  const updatePermissions = (patch: Partial<SetupDraft['runtime']['permissions']>) => updateRuntime({ permissions: { ...draft.runtime.permissions, ...patch } });
  const frequencies = ['1m', '5m', '15m', '30m', '1h'];
  return (
    <div className="space-y-7 border-y border-zinc-800 py-6">
      <div className="grid gap-5 md:grid-cols-2">
        <label className="space-y-2 text-sm"><span className="text-zinc-400">Scan frequency</span><select className="h-10 w-full border border-zinc-800 bg-zinc-950 px-3 text-sm focus:border-amber-500 focus:outline-none" value={draft.runtime.scan_frequency} onChange={(event) => updateRuntime({ scan_frequency: event.target.value })}>{!frequencies.includes(draft.runtime.scan_frequency) && <option value={draft.runtime.scan_frequency}>{draft.runtime.scan_frequency}</option>}{frequencies.map((value) => <option key={value} value={value}>{value}</option>)}</select></label>
        <div className="space-y-3">
          <ToggleRow label="Move source after transfer" checked={draft.runtime.delete_source} onChange={(delete_source) => {
            if (delete_source && !draft.runtime.delete_source) {
              setChecks((state) => ({ ...state, paths: Object.fromEntries(Object.entries(state.paths).filter(([key]) => !key.startsWith('watch:'))) }));
            }
            updateRuntime({ delete_source });
          }} />
          <ToggleRow label="Verify checksums" checked={draft.runtime.verify_checksums} onChange={(verify_checksums) => updateRuntime({ verify_checksums })} />
        </div>
      </div>
      {runtime === 'container' ? (
        <div className="border-l-2 border-cyan-700 bg-cyan-950/10 px-4 py-3"><p className="font-mono text-sm text-cyan-300">container runtime</p><p className="mt-1 text-sm text-zinc-400">Effective UID {uid}, GID {gid}. File ownership is controlled by PUID/PGID.</p></div>
      ) : (
        <fieldset className="border border-zinc-800 p-4"><legend className="px-2 font-mono text-sm text-zinc-300">Transferred file ownership</legend><div className="grid gap-4 sm:grid-cols-2"><LabeledInput label="User" value={draft.runtime.permissions.user} onChange={(user) => updatePermissions({ user })} placeholder="jellyfin" /><LabeledInput label="Group" value={draft.runtime.permissions.group} onChange={(group) => updatePermissions({ group })} placeholder="jellyfin" /><LabeledInput label="File mode" value={draft.runtime.permissions.file_mode} onChange={(file_mode) => updatePermissions({ file_mode })} placeholder="0644" /><LabeledInput label="Directory mode" value={draft.runtime.permissions.dir_mode} onChange={(dir_mode) => updatePermissions({ dir_mode })} placeholder="0755" /></div></fieldset>
      )}
    </div>
  );
}

function ReviewStep({ draft, runtime }: { draft: SetupDraft; runtime: RuntimeKind }) {
  const rows = useMemo(() => [
    ['TV', `${draft.watch.tv.length} incoming / ${draft.libraries.tv.length} library`],
    ['Movies', `${draft.watch.movies.length} incoming / ${draft.libraries.movies.length} library`],
    ['Services', (['sonarr', 'radarr', 'jellyfin'] as ServiceName[]).filter((name) => draft[name].enabled).map(label).join(', ') || 'None'],
    ['AI', draft.ai.enabled ? draft.ai.primary_model : 'Disabled'],
    ['Transfer', draft.runtime.delete_source ? 'Move source' : 'Copy source'],
    ['Runtime', runtime],
  ], [draft, runtime]);
  return <dl className="divide-y divide-zinc-800 border-y border-zinc-800">{rows.map(([term, value]) => <div key={term} className="grid gap-1 py-4 sm:grid-cols-[12rem_1fr]"><dt className="font-mono text-sm text-zinc-500">{term}</dt><dd className="min-w-0 break-words text-sm text-zinc-200">{value}</dd></div>)}</dl>;
}

function SetupComplete({ scanWarning, onDashboard }: { scanWarning?: string; onDashboard: () => void }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-950 p-6 text-zinc-100">
      <div className="w-full max-w-xl border-y border-zinc-800 py-10 text-center">
        <CheckCircle2 className="mx-auto h-10 w-10 text-green-400" />
        <Image src="/p2j-mark.png" alt="P2J" width={150} height={58} className="mx-auto mt-6 h-auto w-[140px]" />
        <h1 className="mt-6 text-2xl font-semibold">Setup complete</h1>
        <p className="mt-2 text-sm text-zinc-400">
          The daemon is running and libraries were indexed (same finish path as the CLI/TUI installer).
        </p>
        {scanWarning ? (
          <p className="mx-auto mt-4 max-w-md text-sm text-amber-300/90">{scanWarning}</p>
        ) : null}
        <div className="mt-7 flex justify-center">
          <Button onClick={onDashboard}>
            Open dashboard
            <ArrowRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

function ToggleRow({ label: text, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <label className="flex h-11 items-center justify-between border-b border-zinc-900 text-sm"><span className="text-zinc-300">{text}</span><Switch checked={checked} onCheckedChange={onChange} /></label>;
}

function LabeledInput({ label: text, value, onChange, placeholder }: { label: string; value: string; onChange: (value: string) => void; placeholder: string }) {
  return <label className="space-y-2 text-sm"><span className="text-zinc-400">{text}</span><Input value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} /></label>;
}

function label(name: string) { return name[0].toUpperCase() + name.slice(1); }
function serviceURL(name: ServiceName) { return name === 'sonarr' ? 'http://sonarr:8989' : name === 'radarr' ? 'http://radarr:7878' : 'http://jellyfin:8096'; }
function stepTitle(step: SetupStep) { return ({ media: 'Media paths', services: 'Connected services', ai: 'AI matching', runtime: 'Runtime behavior', review: 'Review configuration' } as const)[step]; }
