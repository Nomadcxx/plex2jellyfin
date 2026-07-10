'use client';

import { useEffect, useMemo, useState } from 'react';
import { Loader2, Save } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { SettingsSection } from '@/lib/api/client';
import { useSettingsSection, useTestSettingsConnection, useUpdateSettingsSection } from '@/hooks/useSettings';
import { SecretField } from './SecretField';
import { SubsystemReloadStatus } from './SubsystemReloadStatus';
import { Switch } from '@/components/ui/switch';
import { TestConnectionButton } from './TestConnectionButton';

type FieldType = 'text' | 'number' | 'boolean' | 'secret';

export type SettingsField = {
  key: string;
  label: string;
  type?: FieldType;
  placeholder?: string;
};

type Props = {
  section: SettingsSection;
  title: string;
  description: string;
  fields: SettingsField[];
  connectionTest?: 'sonarr' | 'radarr' | 'jellyfin' | 'jellystat';
};

export function SettingsForm({ section, title, description, fields, connectionTest }: Props) {
  const { data, isLoading } = useSettingsSection(section);
  const update = useUpdateSettingsSection(section);
  const testConnection = useTestSettingsConnection(connectionTest || 'sonarr');
  const [draft, setDraft] = useState<Record<string, unknown>>({});

  useEffect(() => {
    if (data) setDraft(data);
  }, [data]);

  const fieldMap = useMemo(() => new Map(fields.map((field) => [field.key, field])), [fields]);

  const setValue = (key: string, value: unknown) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const submit = () => {
    update.mutate(draft, {
      onSuccess: (result) => {
        if (result.reload.ok) toast.success('Settings saved');
        else toast.error('Daemon reload failed; previous config restored');
      },
      onError: () => toast.error('Failed to save settings'),
    });
  };

  const runConnectionTest = () => {
    if (!connectionTest) return;
    testConnection.mutate(draft, {
      onSuccess: (result) => {
        if (result.ok) toast.success(result.version ? `Connected (${result.version})` : 'Connected');
        else toast.error(result.error || 'Connection failed');
      },
      onError: () => toast.error('Connection failed'),
    });
  };

  return (
    <Card className="border-zinc-800 bg-zinc-950/60">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {isLoading ? (
          <div className="flex items-center gap-2 text-sm text-zinc-400">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading settings
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2">
            {fields.map((field) => (
              <label key={field.key} className="space-y-2 text-sm">
                <span className="font-medium text-zinc-300">{field.label}</span>
                <FieldInput field={fieldMap.get(field.key) || field} value={draft[field.key]} onChange={(value) => setValue(field.key, value)} />
              </label>
            ))}
          </div>
        )}

        <SubsystemReloadStatus result={update.data} />

        <div className="flex flex-wrap gap-3">
          {connectionTest && (
            <TestConnectionButton pending={testConnection.isPending} onClick={runConnectionTest} />
          )}
          <Button onClick={submit} disabled={update.isPending || isLoading}>
            {update.isPending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
            Save
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function FieldInput({
  field,
  value,
  onChange,
}: {
  field: SettingsField;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  if (field.type === 'boolean') {
    return (
      <div className="flex items-center gap-2 pt-1">
        <Switch
          checked={Boolean(value)}
          onCheckedChange={(checked) => onChange(checked)}
        />
      </div>
    );
  }
  if (field.type === 'number') {
    return (
      <Input
        type="number"
        value={typeof value === 'number' ? value : ''}
        onChange={(event) => onChange(Number(event.target.value))}
        placeholder={field.placeholder}
      />
    );
  }
  if (field.type === 'secret') {
    return (
      <SecretField
        value={typeof value === 'string' ? value : ''}
        onChange={onChange}
        placeholder={field.placeholder}
      />
    );
  }
  return (
    <Input
      value={typeof value === 'string' ? value : ''}
      onChange={(event) => onChange(event.target.value)}
      placeholder={field.placeholder}
    />
  );
}

