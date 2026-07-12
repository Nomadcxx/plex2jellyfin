'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';
import { PluginStatusPanel } from '@/components/settings/PluginStatusPanel';

export default function JellyfinSettingsPage() {
  return (
    <div className="space-y-6">
      <SettingsSectionPage
        section="jellyfin"
        title="Jellyfin"
        description="Jellyfin connection, playback safety, and webhook secrets."
        connectionTest="jellyfin"
        fields={[
          { key: 'enabled', label: 'Enabled', type: 'boolean' },
          { key: 'url', label: 'URL', placeholder: 'http://localhost:8096' },
          { key: 'api_key', label: 'API key', type: 'secret' },
          { key: 'notify_on_import', label: 'Notify on import', type: 'boolean' },
          { key: 'playback_safety', label: 'Playback safety', type: 'boolean' },
          { key: 'webhook_secret', label: 'Webhook secret', type: 'secret' },
          { key: 'plugin_shared_secret', label: 'Plugin shared secret', type: 'secret' },
        ]}
      />
      <PluginStatusPanel />
    </div>
  );
}
