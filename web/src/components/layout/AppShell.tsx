import { ScanButton } from '@/components/scan/ScanButton';
import { JobsTray } from './JobsTray';
import { Sidebar } from './Sidebar';

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen bg-zinc-950 text-zinc-100">
      <Sidebar />
      <div className="flex-1 flex flex-col overflow-hidden">
        <header className="flex items-center justify-end gap-3 px-6 py-3 border-b border-zinc-800 bg-zinc-950/80 backdrop-blur">
          <JobsTray />
          <ScanButton />
        </header>
        <main className="flex-1 p-6 overflow-auto">
          {children}
        </main>
      </div>
    </div>
  );
}
