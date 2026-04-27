'use client';

import { useState } from 'react';
import { Loader2, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { PreflightResult } from '@/lib/api/client';

type Props = {
  title: string;
  description: string;
  paths: string[];
  loading?: boolean;
  adding?: boolean;
  removing?: boolean;
  onAdd: (path: string) => Promise<unknown>;
  onRemove: (index: number) => Promise<unknown>;
  preflight: (path: string) => Promise<PreflightResult>;
};

export function PathListEditor({
  title,
  description,
  paths,
  loading,
  adding,
  removing,
  onAdd,
  onRemove,
  preflight,
}: Props) {
  const [draft, setDraft] = useState('');
  const [check, setCheck] = useState<PreflightResult | null>(null);

  const add = async () => {
    const path = draft.trim();
    if (!path) return;
    const result = await preflight(path);
    setCheck(result);
    if (!result.readable) {
      toast.error('Path is not readable');
      return;
    }
    await onAdd(path);
    setDraft('');
    toast.success('Path added');
  };

  return (
    <Card className="border-zinc-800 bg-zinc-950/60">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex gap-2">
          <Input value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="/media/path" />
          <Button onClick={add} disabled={adding || !draft.trim()}>
            {adding ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
          </Button>
        </div>

        {check?.warnings?.length ? (
          <div className="rounded-md border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-sm text-amber-200">
            {check.warnings.join('; ')}
          </div>
        ) : null}

        {loading ? (
          <div className="flex items-center gap-2 text-sm text-zinc-400">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading paths
          </div>
        ) : paths.length === 0 ? (
          <div className="rounded-md border border-zinc-800 px-3 py-4 text-sm text-zinc-500">No paths configured.</div>
        ) : (
          <div className="space-y-2">
            {paths.map((path, index) => (
              <div key={`${path}-${index}`} className="flex items-center justify-between gap-3 rounded-md border border-zinc-800 px-3 py-2">
                <span className="min-w-0 truncate font-mono text-sm text-zinc-300">{path}</span>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  disabled={removing}
                  aria-label="Remove path"
                  onClick={async () => {
                    await onRemove(index);
                    toast.success('Path removed');
                  }}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

