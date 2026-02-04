# Auth, Settings, and Build Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement authentication system, read-only settings page, and static export build integration for embedding web dashboard in Go binary.

**Architecture:**
- Authentication uses React Query for state management with cookie-based sessions
- AuthGuard wraps root layout to protect routes, redirects to /login when needed
- Settings page displays real data from available endpoints (/ai/settings, /media-managers) and placeholders for missing ones
- Static export configuration disables server features while maintaining client-side routing with SPA fallback

**Tech Stack:** Next.js 14.2, React Query, shadcn/ui components, Zustand (for UI state only)

---

## PHASE 6: Authentication & Settings

### Task 1: Add shadcn label, switch, and select components

**Files:**
- Create: `web/src/components/ui/label.tsx`
- Create: `web/src/components/ui/switch.tsx`
- Create: `web/src/components/ui/select.tsx`

**Step 1: Add label component**

Run: `cd web && npx shadcn@latest add label`
Expected: label.tsx created in components/ui/

**Step 2: Add switch component**

Run: `cd web && npx shadcn@latest add switch`
Expected: switch.tsx created in components/ui/

**Step 3: Add select component**

Run: `cd web && npx shadcn@latest add select`
Expected: select.tsx created in components/ui/ (includes Select, SelectTrigger, SelectContent, SelectItem, SelectValue)

**Step 4: Verify components exist**

Run: `ls -la web/src/components/ui/ | grep -E 'label|switch|select'`
Expected: Three new files present

**Step 5: Commit**

```bash
git add web/src/components/ui/
git commit -m "feat: add shadcn label, switch, and select components"
```

---

### Task 2: Create auth hooks with React Query

**Files:**
- Modify: `web/src/hooks/useDashboard.ts`

**Step 1: Read existing useDashboard.ts structure**

Run: `cat web/src/hooks/useDashboard.ts`
Expected: See existing dashboard hooks pattern

**Step 2: Add auth status hook to useDashboard.ts**

Add to web/src/hooks/useDashboard.ts:

```typescript
export function useAuthStatus() {
  return useQuery({
    queryKey: ['auth', 'status'],
    queryFn: async () => {
      const response = await fetch('/api/auth/status', {
        credentials: 'include',
      });
      if (!response.ok) {
        throw new Error('Failed to check auth status');
      }
      return response.json() as Promise<components['schemas']['AuthStatus']>;
    },
    staleTime: 1000 * 60, // 1 minute
    retry: false,
  });
}
```

**Step 3: Add login mutation hook to useDashboard.ts**

Add to web/src/hooks/useDashboard.ts:

```typescript
export function useLogin() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (password: string) => {
      const response = await fetch('/api/auth/login', {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ password }),
      });

      if (!response.ok) {
        throw new Error('Invalid password');
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['auth'] });
    },
  });
}
```

**Step 4: Add logout mutation hook to useDashboard.ts**

Add to web/src/hooks/useDashboard.ts:

```typescript
export function useLogout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const response = await fetch('/api/auth/logout', {
        method: 'POST',
        credentials: 'include',
      });

      if (!response.ok) {
        throw new Error('Logout failed');
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['auth'] });
      window.location.href = '/login';
    },
  });
}
```

**Step 5: Commit**

```bash
git add web/src/hooks/useDashboard.ts
git commit -m "feat: add auth hooks (useAuthStatus, useLogin, useLogout)"
```

---

### Task 3: Create LoginForm component

**Files:**
- Create: `web/src/components/auth/LoginForm.tsx`

**Step 1: Create LoginForm component directory**

Run: `mkdir -p web/src/components/auth`
Expected: auth directory created

**Step 2: Create LoginForm.tsx**

Create web/src/components/auth/LoginForm.tsx:

```typescript
'use client';

import { useState } from 'react';
import { useLogin } from '@/hooks/useDashboard';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Loader2 } from 'lucide-react';

export function LoginForm() {
  const [password, setPassword] = useState('');
  const login = useLogin();
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!password.trim()) {
      setError('Password is required');
      return;
    }

    try {
      await login.mutateAsync(password);
    } catch (err) {
      setError('Invalid password');
    }
  };

  return (
    <Card className="w-full max-w-md">
      <CardHeader>
        <CardTitle>Sign in to JellyWatch</CardTitle>
        <CardDescription>Enter your password to access the dashboard</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Input
              type="password"
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={login.isPending}
              autoFocus
            />
            {error && <p className="text-sm text-destructive">{error}</p>}
          </div>
          <Button type="submit" className="w-full" disabled={login.isPending}>
            {login.isPending ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Signing in...
              </>
            ) : (
              'Sign in'
            )}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
```

**Step 3: Commit**

```bash
git add web/src/components/auth/LoginForm.tsx
git commit -m "feat: add LoginForm component with password input and error handling"
```

---

### Task 4: Create AuthGuard component

**Files:**
- Create: `web/src/components/auth/AuthGuard.tsx`

**Step 1: Create AuthGuard.tsx**

Create web/src/components/auth/AuthGuard.tsx:

```typescript
'use client';

import { useEffect } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStatus } from '@/hooks/useDashboard';
import { Loader2 } from 'lucide-react';

interface AuthGuardProps {
  children: React.ReactNode;
}

export function AuthGuard({ children }: AuthGuardProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { data: authStatus, isLoading, isError } = useAuthStatus();

  useEffect(() => {
    // If auth is enabled and not authenticated, redirect to login
    // Skip redirect if already on login page
    if (!isLoading && !isError && authStatus) {
      if (authStatus.enabled && !authStatus.authenticated && pathname !== '/login') {
        router.push('/login');
      }
    }
  }, [authStatus, isLoading, isError, pathname, router]);

  // Show loading while checking auth status
  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // If on login page, just render children (don't check auth)
  if (pathname === '/login') {
    return <>{children}</>;
  }

  // If auth is disabled, allow access
  if (!isError && authStatus && !authStatus.enabled) {
    return <>{children}</>;
  }

  // If authenticated, render children
  if (!isError && authStatus && authStatus.authenticated) {
    return <>{children}</>;
  }

  // Default: show nothing (will redirect via useEffect)
  return null;
}
```

**Step 2: Commit**

```bash
git add web/src/components/auth/AuthGuard.tsx
git commit -m "feat: add AuthGuard component to protect routes with redirect to /login"
```

---

### Task 5: Create login page

**Files:**
- Create: `web/src/app/login/page.tsx`

**Step 1: Create login page directory**

Run: `mkdir -p web/src/app/login`
Expected: login directory created

**Step 2: Create page.tsx**

Create web/src/app/login/page.tsx:

```typescript
'use client';

import { LoginForm } from '@/components/auth/LoginForm';

export default function LoginPage() {
  return (
    <div className="flex items-center justify-center min-h-screen bg-background">
      <LoginForm />
    </div>
  );
}
```

**Step 3: Commit**

```bash
git add web/src/app/login/page.tsx
git commit -m "feat: add login page with LoginForm"
```

---

### Task 6: Update root layout to wrap with AuthGuard

**Files:**
- Modify: `web/src/app/layout.tsx`

**Step 1: Update layout.tsx to import and use AuthGuard**

Replace web/src/app/layout.tsx content with:

```typescript
import type { Metadata } from 'next';
import { Inter } from 'next/font/google';
import './globals.css';
import { Providers } from './providers';
import { AuthGuard } from '@/components/auth/AuthGuard';

const inter = Inter({ subsets: ['latin'] });

export const metadata: Metadata = {
  title: 'JellyWatch',
  description: 'Media library organization dashboard',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body className={inter.className}>
        <Providers>
          <AuthGuard>{children}</AuthGuard>
        </Providers>
      </body>
    </html>
  );
}
```

**Step 2: Commit**

```bash
git add web/src/app/layout.tsx
git commit -m "feat: wrap root layout with AuthGuard for route protection"
```

---

### Task 7: Update Sidebar to add logout button and Settings link

**Files:**
- Modify: `web/src/components/layout/Sidebar.tsx`

**Step 1: Read current Sidebar.tsx to understand navigation structure**

Run: `cat web/src/components/layout/Sidebar.tsx`
Expected: See navigation array with 5 items

**Step 2: Add Settings to navigation and import new icons**

Update web/src/components/layout/Sidebar.tsx:

Change import line to:
```typescript
import { LayoutDashboard, Copy, Download, Activity, FolderSync, Menu, Settings, LogOut } from 'lucide-react';
```

Update navigation array to include Settings:
```typescript
const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Duplicates', href: '/duplicates', icon: Copy },
  { name: 'Queue', href: '/queue', icon: Download },
  { name: 'Activity', href: '/activity', icon: Activity },
  { name: 'Consolidation', href: '/consolidation', icon: FolderSync },
  { name: 'Settings', href: '/settings', icon: Settings },
];
```

**Step 3: Add logout button to Sidebar bottom section**

Add logout functionality after the closing </nav> tag (around line 86):

```typescript
          <div className="mt-auto">
            <Button
              variant="ghost"
              className="w-full justify-start gap-3"
              onClick={handleLogout}
            >
              <LogOut className="h-4 w-4" />
              {!sidebarCollapsed && <span className="text-sm">Logout</span>}
            </Button>
          </div>
```

**Step 4: Import useLogout hook at top of Sidebar component**

After existing imports, add:
```typescript
import { useLogout, useAuthStatus } from '@/hooks/useDashboard';
```

**Step 5: Add logout handler function inside Sidebar component**

After the component function definition, add:
```typescript
export function Sidebar() {
  const pathname = usePathname();
  const { sidebarCollapsed, toggleSidebar } = useUIStore();
  const logout = useLogout();
  const { data: authStatus } = useAuthStatus();

  const handleLogout = async () => {
    await logout.mutateAsync();
  };

  // ... rest of component
```

**Step 6: Only show logout button if auth is enabled and authenticated**

Update the logout button to conditionally render:

```typescript
          {authStatus?.enabled && authStatus.authenticated && (
            <div className="mt-auto pt-4 border-t border-border">
              <Button
                variant="ghost"
                className="w-full justify-start gap-3"
                onClick={handleLogout}
                disabled={logout.isPending}
              >
                <LogOut className="h-4 w-4" />
                {!sidebarCollapsed && <span className="text-sm">Logout</span>}
              </Button>
            </div>
          )}
```

**Step 7: Commit**

```bash
git add web/src/components/layout/Sidebar.tsx
git commit -m "feat: add Settings link and logout button to Sidebar"
```

---

### Task 8: Create ConfigSection component for Settings page

**Files:**
- Create: `web/src/components/settings/ConfigSection.tsx`

**Step 1: Create settings component directory**

Run: `mkdir -p web/src/components/settings`
Expected: settings directory created

**Step 2: Create ConfigSection.tsx**

Create web/src/components/settings/ConfigSection.tsx:

```typescript
'use client';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';

interface ConfigSectionProps {
  title: string;
  children: React.ReactNode;
  description?: string;
  badge?: string;
}

export function ConfigSection({ title, children, description, badge }: ConfigSectionProps) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle className="text-base">{title}</CardTitle>
          {badge && <Badge variant="secondary">{badge}</Badge>}
        </div>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}
```

**Step 3: Commit**

```bash
git add web/src/components/settings/ConfigSection.tsx
git commit -m "feat: add ConfigSection component for Settings page sections"
```

---

### Task 9: Create Settings page with actual and placeholder data

**Files:**
- Create: `web/src/app/settings/page.tsx`

**Step 1: Create settings page directory**

Run: `mkdir -p web/src/app/settings`
Expected: settings directory created

**Step 2: Create page.tsx with full settings implementation**

Create web/src/app/settings/page.tsx:

```typescript
'use client';

import { useAISettings, useMediaManagers } from '@/hooks/useDashboard';
import { ConfigSection } from '@/components/settings/ConfigSection';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Separator } from '@/components/ui/separator';
import { Loader2, Server, Cpu, AlertCircle } from 'lucide-react';

export default function SettingsPage() {
  const { data: aiSettings, isLoading: aiLoading } = useAISettings();
  const { data: mediaManagers, isLoading: managersLoading } = useMediaManagers();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
        <p className="text-muted-foreground mt-2">
          View and manage your JellyWatch configuration
        </p>
      </div>

      {/* AI Settings - Real Data */}
      <ConfigSection title="AI Configuration" description="LLM provider settings">
        {aiLoading ? (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading AI settings...
          </div>
        ) : aiSettings ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium">AI Enabled</p>
                <p className="text-sm text-muted-foreground">
                  Use AI for low-confidence file parsing
                </p>
              </div>
              <Switch checked={aiSettings.enabled} disabled />
            </div>
            <Separator />
            <div className="space-y-2">
              <p className="text-sm font-medium">Default Provider</p>
              <p className="text-sm text-muted-foreground">
                {aiSettings.defaultProvider || 'Not configured'}
              </p>
            </div>
            <Separator />
            <div className="space-y-2">
              <p className="text-sm font-medium">Confidence Threshold</p>
              <p className="text-sm text-muted-foreground">
                {(aiSettings.confidenceThreshold ?? 0.8).toFixed(2)}
              </p>
            </div>
            <Separator />
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium">Auto-Apply Suggestions</p>
                <p className="text-sm text-muted-foreground">
                  Automatically apply AI suggestions above threshold
                </p>
              </div>
              <Switch checked={aiSettings.autoApply ?? false} disabled />
            </div>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">Unable to load AI settings</p>
        )}
      </ConfigSection>

      {/* Media Managers - Real Data */}
      <ConfigSection title="Media Managers" description="Sonarr and Radarr integration">
        {managersLoading ? (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading media managers...
          </div>
        ) : mediaManagers && mediaManagers.length > 0 ? (
          <div className="space-y-3">
            {mediaManagers.map((manager) => (
              <Card key={manager.id}>
                <CardContent className="pt-4">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <Server className="h-5 w-5 text-muted-foreground" />
                      <div>
                        <p className="font-medium">{manager.name}</p>
                        <p className="text-sm text-muted-foreground">
                          Type: {manager.type}
                        </p>
                      </div>
                    </div>
                    <Badge variant={manager.enabled ? 'default' : 'secondary'}>
                      {manager.enabled ? 'Enabled' : 'Disabled'}
                    </Badge>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No media managers configured</p>
        )}
      </ConfigSection>

      {/* Watch Directories - Placeholder */}
      <ConfigSection
        title="Watch Directories"
        description="Folders monitored for new downloads"
        badge="Coming Soon"
      >
        <div className="flex items-start gap-3 text-muted-foreground">
          <AlertCircle className="h-5 w-5 shrink-0" />
          <div className="text-sm">
            <p>Watch directory configuration will be available in a future update.</p>
            <p className="mt-1">Currently configured in ~/.config/jellywatch/config.toml</p>
          </div>
        </div>
      </ConfigSection>

      {/* Library Paths - Placeholder */}
      <ConfigSection
        title="Library Paths"
        description="Target locations for organized media"
        badge="Coming Soon"
      >
        <div className="flex items-start gap-3 text-muted-foreground">
          <AlertCircle className="h-5 w-5 shrink-0" />
          <div className="text-sm">
            <p>Library path configuration will be available in a future update.</p>
            <p className="mt-1">Currently configured in ~/.config/jellywatch/config.toml</p>
          </div>
        </div>
      </ConfigSection>

      {/* General Options - Placeholder */}
      <ConfigSection
        title="General Options"
        description="Application-wide settings"
        badge="Coming Soon"
      >
        <div className="flex items-start gap-3 text-muted-foreground">
          <AlertCircle className="h-5 w-5 shrink-0" />
          <div className="text-sm">
            <p>General options configuration will be available in a future update.</p>
            <p className="mt-1">Currently configured in ~/.config/jellywatch/config.toml</p>
          </div>
        </div>
      </ConfigSection>
    </div>
  );
}
```

**Step 3: Commit**

```bash
git add web/src/app/settings/page.tsx
git commit -m "feat: add Settings page with real and placeholder config sections"
```

---

## PHASE 7: Testing & Build Integration

### Task 10: Update next.config.js for static export

**Files:**
- Modify: `web/next.config.js`

**Step 1: Update next.config.js for static export**

Replace web/next.config.js content with:

```javascript
/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export',
  distDir: 'dist',
  assetPrefix: '.',
  images: {
    unoptimized: true,
  },

  async rewrites() {
    return process.env.NODE_ENV === 'development'
      ? [
          {
            source: '/api/:path*',
            destination: 'http://localhost:8686/api/:path*',
          },
        ]
      : [];
  },
};

module.exports = nextConfig;
```

**Step 2: Commit**

```bash
git add web/next.config.js
git commit -m "feat: configure Next.js for static export (output: 'export')"
```

---

### Task 11: Test static export build

**Files:**
- None (verification step)

**Step 1: Clean existing build artifacts**

Run: `cd web && rm -rf .next out dist`
Expected: Directories removed

**Step 2: Run production build**

Run: `cd web && npm run build 2>&1 | tee build.log`
Expected: Build completes successfully, no errors

**Step 3: Verify dist directory was created**

Run: `ls -la web/dist/`
Expected: dist directory exists with index.html and assets

**Step 4: Check build log for errors**

Run: `grep -i 'error\|warn' web/build.log | grep -v 'ExperimentalWarning'`
Expected: No critical errors or warnings

**Step 5: Verify key pages exist in output**

Run: `ls web/dist/ | grep -E 'index|login|settings|duplicates|queue|activity|consolidation'`
Expected: Multiple HTML files for different pages

**Step 6: Run TypeScript type check**

Run: `cd web && npm run typecheck`
Expected: No type errors

**Note:** Don't commit yet - we'll test more in Task 12

---

### Task 12: Verify all pages build and work with static export

**Files:**
- None (verification step)

**Step 1: Check if all pages are using 'use client' where needed**

Run: `grep -r "^'use client'" web/src/app/ | wc -l`
Expected: At least 6 client pages (login, settings, duplicates, queue, activity, consolidation)

**Step 2: Check for server components that might break static export**

Run: `grep -r "fetch.*localhost" web/src/ | grep -v "rewrites\|localhost:8686"`
Expected: No direct localhost fetches outside of API rewrites

**Step 3: Test build again to ensure everything works**

Run: `cd web && npm run build 2>&1 | tail -20`
Expected: Build succeeds, shows "Creating an optimized production build..." and "Route (app)" listings

**Step 4: Verify no hydration errors in console**

Run: `cat web/build.log | grep -i hydrat`
Expected: No hydration-related warnings or errors

**Step 5: Check that all required shadcn components are present**

Run: `ls web/src/components/ui/ | wc -l`
Expected: At least 15 components (button, card, input, label, switch, select, badge, tooltip, etc.)

**Step 6: Commit successful build configuration**

```bash
git add web/next.config.js web/build.log
git commit -m "feat: static export build verified and working"
```

---

### Task 13: Update root Makefile with web build targets

**Files:**
- Modify: `Makefile`

**Step 1: Read existing Makefile structure**

Run: `cat Makefile`
Expected: See existing build targets

**Step 2: Add web build targets to Makefile**

Add these sections to Makefile (after existing targets):

```makefile
# Web build targets
WEB_DIR := web
WEB_DIST := $(WEB_DIR)/dist
EMBED_DIR := embedded

.PHONY: web web-dev web-build web-clean embed

web-dev: ## Run web dev server
	cd $(WEB_DIR) && npm run dev

web-build: ## Build static web assets
	cd $(WEB_DIR) && npm ci && npm run build

web-clean: ## Clean web build
	rm -rf $(WEB_DIST)

embed: web-build ## Build and prepare for embedding
	rm -rf $(EMBED_DIR)/web
	mkdir -p $(EMBED_DIR)/web
	cp -r $(WEB_DIST)/* $(EMBED_DIR)/web/

# Update main build target to include embed
build: embed ## Full build with web assets
	go build -o jellywatchd ./cmd/jellywatchd
```

**Step 3: Verify make targets are correct**

Run: `make help | grep -E 'web|embed'`
Expected: See web-dev, web-build, web-clean, and embed targets listed

**Step 4: Test web-build target**

Run: `make web-build`
Expected: Clean web build in web/dist/

**Step 5: Test embed target**

Run: `make embed`
Expected: Files copied from web/dist/ to embedded/web/

**Step 6: Verify embed output**

Run: `ls -la embedded/web/ | head -20`
Expected: embedded/web/ contains all built files

**Step 7: Commit**

```bash
git add Makefile embedded/
git commit -m "feat: add web build targets to Makefile (web-dev, web-build, web-clean, embed)"
```

---

### Task 14: Final verification and cleanup

**Files:**
- None (verification step)

**Step 1: Run full build to verify everything works**

Run: `make build`
Expected: Complete build including web assets

**Step 2: Verify embedded/web/ contains all pages**

Run: `find embedded/web/ -name '*.html' | head -10`
Expected: Multiple HTML files including index.html, login.html, settings.html, etc.

**Step 3: Check that all necessary assets are embedded**

Run: `ls embedded/web/_next/static/ 2>/dev/null | head -5 || ls embedded/web/ | grep -E 'css|js|media'`
Expected: CSS, JS, and media assets present

**Step 4: Run final type check**

Run: `cd web && npm run typecheck`
Expected: No type errors

**Step 5: Verify all auth components exist**

Run: `ls -la web/src/components/auth/`
Expected: LoginForm.tsx and AuthGuard.tsx present

**Step 6: Verify all settings components exist**

Run: `ls -la web/src/components/settings/`
Expected: ConfigSection.tsx present

**Step 7: Check git status for uncommitted changes**

Run: `git status`
Expected: No uncommitted changes

**Step 8: Create summary commit if needed**

If there are any remaining changes:
```bash
git add .
git commit -m "feat: complete auth, settings, and build integration"
```

---

## TESTING CHECKLIST

After completing all tasks, verify:

- [ ] Login page loads at /login
- [ ] Login form accepts password and shows error on wrong password
- [ ] AuthGuard protects all routes when auth enabled
- [ ] Logout button appears when authenticated
- [ ] Settings page displays AI settings (real data)
- [ ] Settings page displays Media Managers (real data)
- [ ] Settings page shows "Coming Soon" badges for missing sections
- [ ] Static export builds without errors (npm run build)
- [ ] All pages have 'use client' where needed
- [ ] Makefile web targets work correctly
- [ ] make build creates complete embedded/web/ directory
- [ ] TypeScript compilation passes (npm run typecheck)

---

## BACKEND INTEGRATION NOTES

This implementation is frontend-only. Backend changes needed for full auth:

1. **API Endpoints** (already in OpenAPI spec):
   - `/api/auth/status` - Check if auth enabled and current session
   - `/api/auth/login` - POST with password, sets session cookie
   - `/api/auth/logout` - POST, clears session cookie

2. **Static File Serving** (Go backend):
   - Serve embedded files from `embedded/web/`
   - SPA fallback: serve index.html for non-API routes
   - Cookie-based session management

3. **Config Endpoint** (missing, placeholder used):
   - `/api/config` - Return full configuration for Settings page
   - Currently using `/api/ai/settings` and `/api/media-managers`

4. **Session Middleware** (backend):
   - Cookie middleware for auth
   - Session storage (in-memory or database)
   - Password validation from config
