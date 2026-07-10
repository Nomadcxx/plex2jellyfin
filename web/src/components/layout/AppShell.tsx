import { ScanButton } from '@/components/scan/ScanButton';
import { JobsTray } from './JobsTray';
import { MobileNav, Sidebar } from './Sidebar';
import { TermPrompt } from './TermPrompt';

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen bg-zinc-950 text-zinc-100">
      <Sidebar />
      <div className="flex-1 flex flex-col overflow-hidden">
        <header className="flex items-center justify-between gap-3 px-6 py-3 border-b border-zinc-800 bg-zinc-950/80 backdrop-blur">
          <TermPrompt />
          <div className="flex items-center gap-3">
            <JobsTray />
            <ScanButton />
          </div>
        </header>
        <main className="flex-1 p-4 pb-24 overflow-auto md:p-6 md:pb-6">
          {children}
        </main>
      </div>
      <MobileNav />
    </div>
  );
}
