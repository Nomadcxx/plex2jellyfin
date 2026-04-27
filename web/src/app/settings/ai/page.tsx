'use client';

import { SettingsSectionPage } from '@/components/settings/SettingsSectionPage';

export default function AISettingsPage() {
  return (
    <SettingsSectionPage
      section="ai"
      title="AI"
      description="Ollama model and enhancement settings."
      fields={[
        { key: 'enabled', label: 'Enabled', type: 'boolean' },
        { key: 'ollama_endpoint', label: 'Ollama endpoint', placeholder: 'http://localhost:11434' },
        { key: 'model', label: 'Model' },
        { key: 'fallback_model', label: 'Fallback model' },
        { key: 'confidence_threshold', label: 'Confidence threshold', type: 'number' },
        { key: 'auto_trigger_threshold', label: 'Auto-trigger threshold', type: 'number' },
        { key: 'timeout_seconds', label: 'Timeout seconds', type: 'number' },
        { key: 'hourly_limit', label: 'Hourly limit', type: 'number' },
        { key: 'daily_limit', label: 'Daily limit', type: 'number' },
        { key: 'enhancement_interval_seconds', label: 'Enhancement interval seconds', type: 'number' },
        { key: 'cache_enabled', label: 'Cache enabled', type: 'boolean' },
        { key: 'auto_resolve_risky', label: 'Auto-resolve risky matches', type: 'boolean' },
      ]}
    />
  );
}

