'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';

export default function OptionsSettingsPage() {
  return (
    <SettingsSectionPage
      section="options"
      title="Options"
      description="Core processing behavior for file moves and verification."
      fields={[
        { key: 'dry_run', label: 'Dry run', type: 'boolean' },
        { key: 'verify_checksums', label: 'Verify checksums', type: 'boolean' },
        { key: 'delete_source', label: 'Delete source', type: 'boolean' },
      ]}
    />
  );
}

