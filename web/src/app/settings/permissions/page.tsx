'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';

export default function PermissionsSettingsPage() {
  return (
    <SettingsSectionPage
      section="permissions"
      title="Permissions"
      description="Optional ownership and mode settings for organized files."
      fields={[
        { key: 'user', label: 'User' },
        { key: 'group', label: 'Group' },
        { key: 'file_mode', label: 'File mode', placeholder: '0644' },
        { key: 'dir_mode', label: 'Directory mode', placeholder: '0755' },
      ]}
    />
  );
}

