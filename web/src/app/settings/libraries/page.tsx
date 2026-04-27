'use client';

import { PathListEditor } from '@/components/settings/PathListEditor';
import { preflightPath } from '@/lib/api/client';
import { useAddSettingsPath, useRemoveSettingsPath, useSettingsPaths } from '@/hooks/useSettings';

export default function LibrariesSettingsPage() {
  const tv = useSettingsPaths('libraries', 'tv');
  const movies = useSettingsPaths('libraries', 'movies');
  const addTV = useAddSettingsPath('libraries', 'tv');
  const addMovies = useAddSettingsPath('libraries', 'movies');
  const removeTV = useRemoveSettingsPath('libraries', 'tv');
  const removeMovies = useRemoveSettingsPath('libraries', 'movies');

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Libraries</h1>
        <p className="mt-1 text-sm text-zinc-400">Destination folders for organized media.</p>
      </div>
      <div className="grid gap-5 xl:grid-cols-2">
        <PathListEditor
          title="TV Libraries"
          description="Organized episode destinations."
          paths={tv.data ?? []}
          loading={tv.isLoading}
          adding={addTV.isPending}
          removing={removeTV.isPending}
          onAdd={(path) => addTV.mutateAsync(path)}
          onRemove={(index) => removeTV.mutateAsync(index)}
          preflight={(path) => preflightPath(path, 'library')}
        />
        <PathListEditor
          title="Movie Libraries"
          description="Organized movie destinations."
          paths={movies.data ?? []}
          loading={movies.isLoading}
          adding={addMovies.isPending}
          removing={removeMovies.isPending}
          onAdd={(path) => addMovies.mutateAsync(path)}
          onRemove={(index) => removeMovies.mutateAsync(index)}
          preflight={(path) => preflightPath(path, 'library')}
        />
      </div>
    </div>
  );
}

