'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';

export default function LoggingSettingsPage() {
  return (
    <SettingsSectionPage
      section="logging"
      title="Logging"
      description="Runtime log level and rotation policy."
      fields={[
        { key: 'level', label: 'Level', placeholder: 'info' },
        { key: 'file', label: 'File' },
        { key: 'max_size_mb', label: 'Max size MB', type: 'number' },
        { key: 'max_backups', label: 'Max backups', type: 'number' },
        { key: 'max_age_days', label: 'Max age days', type: 'number' },
        { key: 'compress', label: 'Compress rotated logs', type: 'boolean' },
      ]}
    />
  );
}

