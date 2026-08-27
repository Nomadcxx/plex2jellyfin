'use client';

import { PathListEditor } from '@/components/settings/PathListEditor';
import { preflightPath } from '@/lib/api/client';
import { useAddSettingsPath, useRemoveSettingsPath, useSettingsPaths } from '@/hooks/useSettings';

const panels = [
  {
    kind: 'tv',
    title: 'TV',
    description: 'Incoming episode folders.',
  },
  {
    kind: 'movies',
    title: 'Movies',
    description: 'Incoming movie folders.',
  },
  {
    kind: 'seeding_tv',
    title: 'TV seeding completions',
    description: 'Completed episode downloads kept for seeding. Sources stay in place; imports hardlink when the filesystem allows it.',
  },
  {
    kind: 'seeding_movies',
    title: 'Movie seeding completions',
    description: 'Completed movie downloads kept for seeding. Sources stay in place; imports hardlink when the filesystem allows it.',
  },
] as const;

function PathsPanel({ kind, title, description }: (typeof panels)[number]) {
  const paths = useSettingsPaths('paths', kind);
  const add = useAddSettingsPath('paths', kind);
  const remove = useRemoveSettingsPath('paths', kind);

  return (
    <PathListEditor
      title={title}
      description={description}
      paths={paths.data ?? []}
      loading={paths.isLoading}
      adding={add.isPending}
      removing={remove.isPending}
      onAdd={(path) => add.mutateAsync(path)}
      onRemove={(index) => remove.mutateAsync(index)}
      preflight={(path) => preflightPath(path, 'watch')}
    />
  );
}

export default function PathsSettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Watch Paths</h1>
        <p className="mt-1 text-sm text-zinc-400">
          Folders watched and scanned for incoming media. Keep incomplete downloads outside every path listed here.
        </p>
      </div>
      <div className="grid gap-5 xl:grid-cols-2">
        {panels.map((panel) => <PathsPanel key={panel.kind} {...panel} />)}
      </div>
    </div>
  );
}
