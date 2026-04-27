'use client';

import Link from 'next/link';
import { ArrowRight, Bot, FolderOpen, HardDrive, ListChecks, Radio, Settings, SlidersHorizontal, Wrench } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

const sections = [
  { href: '/settings/paths', title: 'Watch Paths', desc: 'Folders scanned for new media.', icon: FolderOpen },
  { href: '/settings/libraries', title: 'Libraries', desc: 'Destinations for organized movies and TV.', icon: HardDrive },
  { href: '/settings/sonarr', title: 'Sonarr', desc: 'TV manager connection and import notification settings.', icon: Radio },
  { href: '/settings/radarr', title: 'Radarr', desc: 'Movie manager connection and import notification settings.', icon: Radio },
  { href: '/settings/jellyfin', title: 'Jellyfin', desc: 'Server connection, playback safety, and webhooks.', icon: ListChecks },
  { href: '/settings/ai', title: 'AI', desc: 'Ollama parsing and enhancement model settings.', icon: Bot },
  { href: '/settings/options', title: 'Options', desc: 'Transfer and verification behavior.', icon: SlidersHorizontal },
  { href: '/settings/logging', title: 'Logging', desc: 'Runtime log level and rotation settings.', icon: Wrench },
];

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="flex items-center gap-3 text-3xl font-bold">
          <Settings className="h-7 w-7" />
          Settings
        </h1>
        <p className="mt-2 text-zinc-400">Edit local config sections and hot-reload the daemon.</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {sections.map((section) => {
          const Icon = section.icon;
          return (
            <Link key={section.href} href={section.href}>
              <Card className="h-full border-zinc-800 bg-zinc-950/60 transition-colors hover:bg-zinc-900/60">
                <CardHeader>
                  <CardTitle className="flex items-center justify-between gap-3 text-base">
                    <span className="flex items-center gap-2">
                      <Icon className="h-5 w-5 text-zinc-400" />
                      {section.title}
                    </span>
                    <ArrowRight className="h-4 w-4 text-zinc-500" />
                  </CardTitle>
                </CardHeader>
                <CardContent className="text-sm text-zinc-400">{section.desc}</CardContent>
              </Card>
            </Link>
          );
        })}
      </div>
    </div>
  );
}

