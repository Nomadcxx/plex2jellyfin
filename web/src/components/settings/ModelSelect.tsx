'use client';

import { RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';

interface ModelSelectProps {
  value: string;
  onChange: (value: string) => void;
  models: string[];
  loading?: boolean;
  error?: string | null;
  onRefresh?: () => void;
  placeholder?: string;
  allowCustom?: boolean;
}

const CUSTOM_SENTINEL = '__custom__';

export function ModelSelect({
  value,
  onChange,
  models,
  loading,
  error,
  onRefresh,
  placeholder,
  allowCustom = true,
}: ModelSelectProps) {
  const knownModels = models ?? [];
  const isInList = value === '' || knownModels.includes(value);
  const showCustomInput = allowCustom && !isInList;

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <select
          className="flex h-10 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-200 ring-offset-zinc-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400 disabled:cursor-not-allowed disabled:opacity-50"
          value={showCustomInput ? CUSTOM_SENTINEL : value}
          onChange={(e) => {
            if (e.target.value === CUSTOM_SENTINEL) {
              onChange(value && !knownModels.includes(value) ? value : '');
            } else {
              onChange(e.target.value);
            }
          }}
          disabled={loading && knownModels.length === 0}
        >
          <option value="">{placeholder ?? 'Select a model…'}</option>
          {knownModels.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
          {value && !knownModels.includes(value) && !showCustomInput && (
            <option value={value}>{value} (not installed)</option>
          )}
          {allowCustom && (
            <option value={CUSTOM_SENTINEL}>Custom…</option>
          )}
        </select>
        {onRefresh && (
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={onRefresh}
            disabled={loading}
            aria-label="Refresh models"
            title="Refresh model list"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </Button>
        )}
      </div>

      {showCustomInput && (
        <Input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder ?? 'model:tag'}
        />
      )}

      {!showCustomInput && value && !knownModels.includes(value) && (
        <Badge variant="outline" className="border-amber-500/40 text-amber-400">
          {value} (not installed)
        </Badge>
      )}

      {error && (
        <p className="text-xs text-amber-400">
          {error}. Showing previously-saved value.
        </p>
      )}
    </div>
  );
}
