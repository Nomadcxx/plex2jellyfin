'use client';

import Image from 'next/image';
import { useEffect, useMemo, useState } from 'react';
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
import { Progress } from '@/components/ui/progress';
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
import { SetupDraft, SetupIndexEvent, SetupStatus, setupKeys, useApplySetup } from '@/hooks/useSetup';
import {
  emptyChecks,
  hydrateDraft,
  pathCheckKey,
  RuntimeKind,
  ServiceName,
  serviceFingerprint,
  SetupStep,
  splitPathInput,
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
  const [phase, setPhase] = useState<'wizard' | 'indexing' | 'complete'>('wizard');
  const [scanWarning, setScanWarning] = useState('');
  const [indexResult, setIndexResult] = useState<SetupIndexEvent | null>(null);
  const apply = useApplySetup();
  const router = useRouter();
  const queryClient = useQueryClient();
  const current = steps[stepIndex];

  const libraryPaths = useMemo(
    () => [...draft.libraries.tv, ...draft.libraries.movies],
    [draft.libraries.tv, draft.libraries.movies],
  );

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

  if (phase === 'indexing') {
    return (
      <SetupIndexing
        libraries={libraryPaths}
        onDone={(result) => {
          setIndexResult(result);
          if (result.scan_warning) {
            setScanWarning(result.scan_warning);
            toast.warning(result.scan_warning);
          }
          setPhase('complete');
        }}
      />
    );
  }

  if (phase === 'complete') {
    return <SetupComplete scanWarning={scanWarning} result={indexResult} onDashboard={finish} />;
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
            <p className="mt-2 max-w-3xl text-sm text-zinc-400">{stepBlurb(current.id)}</p>
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
                      if (result.plugin_warning) {
                        toast.warning(result.plugin_warning);
                      }
                      if (result.indexing) {
                        setPhase('indexing');
                        return;
                      }
                      if (result.scan_warning) {
                        setScanWarning(result.scan_warning);
                        toast.warning(result.scan_warning);
                      }
                      setPhase('complete');
                    },
                  });
                }}
                disabled={apply.isPending}
              >
                {apply.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                {apply.isPending ? 'Starting daemon…' : 'Apply and start'}
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
      <div className="space-y-2 border-l-2 border-amber-500/40 bg-amber-950/10 px-4 py-3 text-sm text-zinc-300">
        <p>Configure TV, Movies, or both. Each configured type needs an incoming path and a library path.</p>
        <p className="text-zinc-400">
          Add one path at a time, or paste a comma-separated list (same as the CLI). Browser file pickers cannot see server mounts like <span className="font-mono text-zinc-300">/mnt/…</span>, so paths are entered as text and verified on the host.
        </p>
      </div>
      {([
        ['tv', 'TV', Tv],
        ['movies', 'Movies', Film],
      ] as const).map(([media, title, Icon]) => (
        <section key={media} aria-labelledby={`${media}-paths`} className="border-t border-zinc-800 pt-5 first:border-t-0 first:pt-0">
          <div className="mb-4 flex items-center gap-2"><Icon className="h-5 w-5 text-amber-400" /><h2 id={`${media}-paths`} className="font-mono text-base font-semibold">{title}</h2><span className="text-xs text-zinc-600">optional</span></div>
          <div className="grid gap-5 xl:grid-cols-2">
            <PathEditor
              title="Incoming"
              hint="Downloader complete/watch folder (SABnzbd, qBittorrent, NZBGet, etc.)."
              example={media === 'tv' ? '/mnt/NVME3/Sabnzbd/complete/tv' : '/mnt/NVME3/Sabnzbd/complete/movies'}
              icon={FolderInput}
              collection="watch"
              media={media}
              paths={draft.watch[media]}
              writable={draft.runtime.delete_source}
              checks={checks}
              setChecks={setChecks}
              onChange={(paths) => setPath('watch', media, paths)}
            />
            <PathEditor
              title="Library"
              hint="Final library roots Jellyfin/Sonarr/Radarr use. Paste one path or a comma-separated list across drives."
              example={media === 'tv'
                ? '/mnt/STORAGE1/TVSHOWS, /mnt/STORAGE2/TVSHOWS, /mnt/STORAGE3/TVSHOWS'
                : '/mnt/STORAGE1/MOVIES, /mnt/STORAGE2/MOVIES, /mnt/STORAGE3/MOVIES'}
              icon={FolderOutput}
              collection="libraries"
              media={media}
              paths={draft.libraries[media]}
              writable
              checks={checks}
              setChecks={setChecks}
              onChange={(paths) => setPath('libraries', media, paths)}
            />
          </div>
        </section>
      ))}
    </div>
  );
}

function PathEditor({ title, hint, example, icon: Icon, collection, media, paths, writable, checks, setChecks, onChange }: {
  title: string;
  hint: string;
  example: string;
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
  const [checking, setChecking] = useState(false);

  const verify = async (path: string) => {
    const result = await preflightPath(path, collection === 'watch' ? 'watch' : 'library');
    const ok = result.exists && result.is_dir && result.readable && (!writable || result.writable);
    setChecks((state) => ({ ...state, paths: { ...state.paths, [pathCheckKey(collection, media, path)]: ok } }));
    return { path, ok, warnings: result.warnings };
  };

  const addPaths = async () => {
    const candidates = splitPathInput(value);
    if (candidates.length === 0) return;
    setChecking(true);
    const accepted: string[] = [];
    const failed: string[] = [];
    for (const path of candidates) {
      if (paths.includes(path)) continue;
      const result = await verify(path);
      if (result.ok) accepted.push(path);
      else failed.push(path);
    }
    if (accepted.length > 0) {
      onChange([...paths, ...accepted]);
      setValue('');
      toast.success(accepted.length === 1 ? `${accepted[0]} verified` : `Added ${accepted.length} paths`);
    }
    if (failed.length > 0) {
      toast.error(failed.length === 1 ? `${failed[0]} is not accessible` : `${failed.length} paths failed verification`);
    }
    setChecking(false);
  };

  return (
    <fieldset className="min-w-0 border border-zinc-800 p-4">
      <legend className="px-2 font-mono text-sm text-zinc-300"><span className="inline-flex items-center gap-2"><Icon className="h-4 w-4" />{title}</span></legend>
      <p className="mb-3 text-xs leading-relaxed text-zinc-500">{hint}</p>
      <div className="flex gap-2">
        <Input
          value={value}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              void addPaths();
            }
          }}
          placeholder={example}
          aria-label={`Add ${title.toLowerCase()} path`}
        />
        <Button type="button" size="icon" aria-label={`Add ${title.toLowerCase()} path`} disabled={!value.trim() || checking} onClick={() => void addPaths()}>
          {checking ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
        </Button>
      </div>
      <p className="mt-2 font-mono text-[11px] text-zinc-600">One path, or comma-separated like the CLI.</p>
      <div className="mt-3 min-h-10 space-y-2">
        {paths.length === 0 ? <p className="py-2 text-sm text-zinc-600">Not configured</p> : paths.map((path, index) => {
          const state = checks.paths[pathCheckKey(collection, media, path)];
          return (
            <div key={`${path}-${index}`} className="flex min-h-10 items-center gap-2 border-t border-zinc-900 pt-2">
              {state === true ? <CheckCircle2 className="h-4 w-4 shrink-0 text-green-400" /> : state === false ? <AlertTriangle className="h-4 w-4 shrink-0 text-red-400" /> : <Circle className="h-4 w-4 shrink-0 text-zinc-600" />}
              <span className="min-w-0 flex-1 truncate font-mono text-xs text-zinc-300" title={path}>{path}</span>
              <Button type="button" variant="ghost" size="icon" aria-label={`Verify ${path}`} title="Verify path" disabled={checking} onClick={async () => {
                setChecking(true);
                const result = await verify(path);
                setChecking(false);
                result.ok ? toast.success(`${path} verified`) : toast.error(result.warnings?.join('; ') || `${path} is not accessible`);
              }}>{checking ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}</Button>
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
      <p className="pb-5 text-sm text-zinc-400">
        Optional. Connect Sonarr/Radarr so completed downloads can be imported with the right quality profile behavior, and Jellyfin for library refresh plus the companion plugin feedback loop. Test each service before continuing.
      </p>
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
      <p className="text-sm text-zinc-400">
        Most filenames parse without AI. Enable Ollama only for edge cases: low-confidence titles, missing years, or release tags the regex parser cannot fix.
      </p>
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
      <div className="space-y-2 text-sm text-zinc-400">
        <p>Watch folders are monitored live. Scan frequency is a periodic catch-up for anything the watcher missed (default 5m is fine for most libraries).</p>
        <p>Move deletes the download after a successful import; copy keeps the source. Checksums add an integrity check after each transfer (slower; off by default).</p>
      </div>
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
        <fieldset className="border border-zinc-800 p-4">
          <legend className="px-2 font-mono text-sm text-zinc-300">Transferred file ownership</legend>
          <p className="mb-3 text-xs leading-relaxed text-zinc-500">
            Group is the critical setting: Sonarr, Radarr, and Jellyfin need a shared group (and group-writable modes) to rename/upgrade/delete after import. Recommended: group=media (or jellyfin), file_mode=0664, dir_mode=0775.
          </p>
          <div className="grid gap-4 sm:grid-cols-2"><LabeledInput label="User" value={draft.runtime.permissions.user} onChange={(user) => updatePermissions({ user })} placeholder="jellyfin" /><LabeledInput label="Group" value={draft.runtime.permissions.group} onChange={(group) => updatePermissions({ group })} placeholder="media" /><LabeledInput label="File mode" value={draft.runtime.permissions.file_mode} onChange={(file_mode) => updatePermissions({ file_mode })} placeholder="0664" /><LabeledInput label="Directory mode" value={draft.runtime.permissions.dir_mode} onChange={(dir_mode) => updatePermissions({ dir_mode })} placeholder="0775" /></div>
        </fieldset>
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

type LibRow = { path: string; status: 'queued' | 'scanning' | 'done'; files: number };

function softPct(files: number): number {
  if (files <= 0) return 2;
  return Math.min(92, Math.round((1 - 1 / (files / 40 + 1.05)) * 100));
}

function libraryLabel(path: string): string {
  const clean = path.replace(/\/+$/, '');
  const parts = clean.split('/').filter(Boolean);
  if (parts.length >= 2) return `${parts[parts.length - 2]}/${parts[parts.length - 1]}`;
  return parts[parts.length - 1] || path;
}

function SetupIndexing({
  libraries,
  onDone,
}: {
  libraries: string[];
  onDone: (result: SetupIndexEvent) => void;
}) {
  const [rows, setRows] = useState<LibRow[]>(() => libraries.map((path) => ({ path, status: 'queued', files: 0 })));
  const [filesScanned, setFilesScanned] = useState(0);
  const [phase, setPhase] = useState('Starting index…');
  const [streamError, setStreamError] = useState('');
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    setStreamError('');
    setPhase('Starting index…');
    setRows(libraries.map((path) => ({ path, status: 'queued', files: 0 })));
    setFilesScanned(0);

    const es = new EventSource('/api/v1/setup/index/stream');
    let closed = false;
    let baseline = 0;
    let lastDone = 0;
    const fileCounts = libraries.map(() => 0);

    es.onmessage = (ev) => {
      let frame: SetupIndexEvent;
      try {
        frame = JSON.parse(ev.data);
      } catch {
        return;
      }
      if (frame.type === 'status') {
        setPhase(frame.msg || frame.phase || 'Working…');
        return;
      }
      if (frame.type === 'progress') {
        const done = frame.libraries_done ?? 0;
        const totalFiles = frame.files_scanned ?? 0;
        if (done > lastDone) {
          for (let i = lastDone; i < done && i < fileCounts.length; i++) {
            if (fileCounts[i] === 0) fileCounts[i] = Math.max(0, totalFiles - baseline);
          }
          baseline = totalFiles;
          lastDone = done;
        } else if (done < fileCounts.length) {
          fileCounts[done] = Math.max(0, totalFiles - baseline);
        }
        setFilesScanned(totalFiles);
        setRows(libraries.map((path, index) => ({
          path,
          status: index < done ? 'done' : index === done ? 'scanning' : 'queued',
          files: fileCounts[index] ?? 0,
        })));
        return;
      }
      if (frame.type === 'done') {
        closed = true;
        es.close();
        setPhase(frame.msg || 'Complete');
        setRows(libraries.map((path, index) => ({
          path,
          status: 'done',
          files: fileCounts[index] ?? 0,
        })));
        onDone(frame);
        return;
      }
      if (frame.type === 'error') {
        closed = true;
        es.close();
        setStreamError(frame.msg || 'Indexing failed');
        setPhase('Indexing failed');
      }
    };
    es.onerror = () => {
      if (closed) return;
      es.close();
      setStreamError('Lost connection to the indexing stream (often a server write timeout on long scans). Retry to resume, or run: plex2jellyfin scan');
      setPhase('Connection lost');
    };
    return () => {
      closed = true;
      es.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [attempt]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-950 p-6 text-zinc-100">
      <div className="w-full max-w-2xl border-y border-zinc-800 py-10">
        <Image src="/p2j-mark.png" alt="P2J" width={150} height={58} className="mx-auto h-auto w-[140px]" />
        <h1 className="mt-6 text-center text-2xl font-semibold">Initial library scan</h1>
        <p className="mt-2 text-center text-sm text-zinc-400">
          Indexing libraries the same way as the CLI/TUI installer. Daemon is already up.
        </p>
        <p className="mt-4 text-center font-mono text-xs uppercase text-amber-400">{phase}</p>
        {streamError ? (
          <div className="mx-auto mt-4 max-w-lg space-y-3 border border-amber-900/60 bg-amber-950/20 px-4 py-3 text-sm text-amber-100">
            <p>{streamError}</p>
            <div className="flex justify-center">
              <Button type="button" onClick={() => setAttempt((n) => n + 1)}>Retry indexing</Button>
            </div>
          </div>
        ) : null}
        <div className="mt-8 space-y-3">
          {rows.map((row) => {
            const pct = row.status === 'done' ? 100 : row.status === 'scanning' ? softPct(row.files) : 0;
            return (
              <div key={row.path} className="space-y-1.5">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className={`min-w-0 truncate font-mono ${row.status === 'scanning' ? 'font-semibold text-amber-300' : row.status === 'done' ? 'text-green-400' : 'text-zinc-500'}`} title={row.path}>
                    {libraryLabel(row.path)}
                  </span>
                  <span className="shrink-0 font-mono text-xs text-zinc-500">
                    {row.status === 'done' ? `${row.files.toLocaleString()} files` : row.status === 'scanning' ? 'scanning' : 'queued'}
                  </span>
                </div>
                <Progress value={pct} className="h-2 bg-zinc-900 [&>div]:bg-amber-400" />
              </div>
            );
          })}
        </div>
        <p className="mt-6 text-center font-mono text-xs text-zinc-500">
          total files indexed: {filesScanned.toLocaleString()}
        </p>
      </div>
    </div>
  );
}

function SetupComplete({
  scanWarning,
  result,
  onDashboard,
}: {
  scanWarning?: string;
  result?: SetupIndexEvent | null;
  onDashboard: () => void;
}) {
  const duration = result?.duration_ms ? `${(result.duration_ms / 1000).toFixed(1)}s` : null;
  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-950 p-6 text-zinc-100">
      <div className="w-full max-w-xl border-y border-zinc-800 py-10 text-center">
        <CheckCircle2 className="mx-auto h-10 w-10 text-green-400" />
        <Image src="/p2j-mark.png" alt="P2J" width={150} height={58} className="mx-auto mt-6 h-auto w-[140px]" />
        <h1 className="mt-6 text-2xl font-semibold">Setup complete</h1>
        <p className="mt-2 text-sm text-zinc-400">
          The daemon is running{result ? ' and libraries were indexed' : ''}. Open the dashboard when you are ready.
        </p>
        {result && (
          <div className="mx-auto mt-5 max-w-md space-y-1 text-left font-mono text-xs text-zinc-400">
            {duration ? <p>Scan finished in {duration}</p> : null}
            <p>files indexed: {(result.files_scanned ?? 0).toLocaleString()} (added {result.files_added ?? 0}, updated {result.files_updated ?? 0}, skipped {result.files_skipped ?? 0})</p>
            <p>episode rows: {(result.episode_rows ?? 0).toLocaleString()} · movie rows: {(result.movie_rows ?? 0).toLocaleString()}</p>
            <p className="pt-2 text-zinc-500">Services: plex2jellyfin-daemon + plex2jellyfin-web should stay enabled across reboots.</p>
          </div>
        )}
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
function stepTitle(step: SetupStep) {
  return ({ media: 'Media paths', services: 'Connected services', ai: 'AI matching', runtime: 'Runtime behavior', review: 'Review configuration' } as const)[step];
}
function stepBlurb(step: SetupStep) {
  return ({
    media: 'Incoming paths are where downloads land; library paths are where organized media lives.',
    services: 'Wire up Sonarr, Radarr, and Jellyfin when you use them — skip anything you do not need.',
    ai: 'Optional Ollama assist for ambiguous filenames the regex parser cannot fix.',
    runtime: 'How often to catch up, whether to move or copy, and ownership for imported files.',
    review: 'Confirm the draft, then apply. The daemon starts, libraries are indexed, and config ownership is fixed.',
  } as const)[step];
}
