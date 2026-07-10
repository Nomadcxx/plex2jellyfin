'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';

export default function JellystatSettingsPage() {
  return (
    <SettingsSectionPage
      section="jellystat"
      title="Jellystat"
      description="Optional watch statistics from a Jellystat instance. Purely additive: enables dashboard watch cards and per-item play stats."
      connectionTest="jellystat"
      fields={[
        { key: 'enabled', label: 'Enabled', type: 'boolean' },
        { key: 'url', label: 'URL', placeholder: 'http://localhost:3000' },
        { key: 'api_key', label: 'API key', type: 'secret' },
      ]}
    />
  );
}
