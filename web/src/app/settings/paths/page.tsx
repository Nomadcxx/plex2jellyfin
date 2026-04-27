'use client';

import { PathListEditor } from '@/components/settings/PathListEditor';
import { preflightPath } from '@/lib/api/client';
import { useAddSettingsPath, useRemoveSettingsPath, useSettingsPaths } from '@/hooks/useSettings';

export default function PathsSettingsPage() {
  const tv = useSettingsPaths('paths', 'tv');
  const movies = useSettingsPaths('paths', 'movies');
  const addTV = useAddSettingsPath('paths', 'tv');
  const addMovies = useAddSettingsPath('paths', 'movies');
  const removeTV = useRemoveSettingsPath('paths', 'tv');
  const removeMovies = useRemoveSettingsPath('paths', 'movies');

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Watch Paths</h1>
        <p className="mt-1 text-sm text-zinc-400">Folders watched and scanned for incoming media.</p>
      </div>
      <div className="grid gap-5 xl:grid-cols-2">
        <PathListEditor
          title="TV"
          description="Incoming episode folders."
          paths={tv.data ?? []}
          loading={tv.isLoading}
          adding={addTV.isPending}
          removing={removeTV.isPending}
          onAdd={(path) => addTV.mutateAsync(path)}
          onRemove={(index) => removeTV.mutateAsync(index)}
          preflight={(path) => preflightPath(path, 'watch')}
        />
        <PathListEditor
          title="Movies"
          description="Incoming movie folders."
          paths={movies.data ?? []}
          loading={movies.isLoading}
          adding={addMovies.isPending}
          removing={removeMovies.isPending}
          onAdd={(path) => addMovies.mutateAsync(path)}
          onRemove={(index) => removeMovies.mutateAsync(index)}
          preflight={(path) => preflightPath(path, 'watch')}
        />
      </div>
    </div>
  );
}

