export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen bg-zinc-950 text-zinc-100">
      <Sidebar />
      <main className="flex-1 p-6 overflow-auto">
        {children}
      </main>
    </div>
  );
}

import { Sidebar } from './Sidebar';
