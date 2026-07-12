'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';

export default function TMDBSettingsPage() {
  return (
    <SettingsSectionPage
      section="tmdb"
      title="TMDB"
      description="Optional TMDB API key used by the housekeeping verifier to distinguish remakes from genuine duplicates when Jellyfin is unavailable. Free key from themoviedb.org/settings/api."
      fields={[
        { key: 'enabled', label: 'Enabled', type: 'boolean' },
        { key: 'api_key', label: 'API key (v3)', type: 'secret', placeholder: 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' },
      ]}
    />
  );
}
