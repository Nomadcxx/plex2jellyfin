'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';

export default function RadarrSettingsPage() {
  return (
    <SettingsSectionPage
      section="radarr"
      title="Radarr"
      description="Movie manager connection and notification settings."
      connectionTest="radarr"
      fields={[
        { key: 'enabled', label: 'Enabled', type: 'boolean' },
        { key: 'url', label: 'URL', placeholder: 'http://localhost:7878' },
        { key: 'api_key', label: 'API key', type: 'secret' },
        { key: 'notify_on_import', label: 'Notify on import', type: 'boolean' },
      ]}
    />
  );
}

