'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';
import { CompatibilityPanel } from '@/components/settings/CompatibilityPanel';

export default function SonarrSettingsPage() {
  return (
    <div className="space-y-6">
      <SettingsSectionPage
        section="sonarr"
        title="Sonarr"
        description="TV manager connection and notification settings."
        connectionTest="sonarr"
        fields={[
          { key: 'enabled', label: 'Enabled', type: 'boolean' },
          { key: 'url', label: 'URL', placeholder: 'http://localhost:8989' },
          { key: 'api_key', label: 'API key', type: 'secret' },
          { key: 'notify_on_import', label: 'Notify on import', type: 'boolean' },
        ]}
      />
      <CompatibilityPanel service="sonarr" />
    </div>
  );
}
