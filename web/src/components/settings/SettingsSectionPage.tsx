'use client';

import { SettingsField, SettingsForm } from './SettingsForm';
import { SettingsSection } from '@/lib/api/client';

type Props = {
  section: SettingsSection;
  title: string;
  description: string;
  fields: SettingsField[];
  connectionTest?: 'sonarr' | 'radarr' | 'jellyfin';
};

export function SettingsSectionPage(props: Props) {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">{props.title}</h1>
        <p className="mt-1 text-sm text-zinc-400">{props.description}</p>
      </div>
      <SettingsForm {...props} />
    </div>
  );
}

