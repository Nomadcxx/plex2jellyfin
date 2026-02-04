# Phase 6: Auth & Polish - JellyWatch Web Dashboard

> **Parent Plan**: [Web Dashboard Foundation](../2026-01-31-web-dashboard.md)
> **Previous**: [Phase 5: Consolidation](./2026-01-31-web-dashboard-phase-5-consolidation.md)

**Goal**: Implement optional authentication, UI polish, and production readiness.

---

## Phase 6 Tasks

### Task 6.1: Create Auth Hooks

**Files**: `web/src/hooks/useAuth.ts`

```typescript
export function useAuth() {
  const { data, isLoading } = useQuery<AuthStatus>({
    queryKey: ['auth', 'status'],
    queryFn: authApi.getStatus,
    staleTime: Infinity,
  });

  return {
    isLoading,
    authEnabled: data?.enabled ?? false,
    isAuthenticated: data?.authenticated ?? false,
    requiresLogin: data?.enabled && !data?.authenticated,
  };
}

export function useLogin() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (password: string) => authApi.login(password),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['auth', 'status'] });
    },
  });
}
```

**Commit**: `git add web/src/hooks/useAuth.ts && git commit -m "feat: add authentication hooks"`

---

### Task 6.2: Create Login Page

**Files**: `web/src/app/login/page.tsx`

```typescript
export default function LoginPage() {
  const [password, setPassword] = useState('');
  const { mutate: login, isPending, error } = useLogin();
  const router = useRouter();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    login(password, {
      onSuccess: () => router.push('/'),
    });
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <h1 className="text-2xl font-bold">JellyWatch</h1>
          <p className="text-muted-foreground">Enter password to continue</p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Password"
              autoFocus
            />
            {error && (
              <p className="text-sm text-destructive">Invalid password</p>
            )}
            <Button type="submit" className="w-full" disabled={isPending}>
              {isPending ? 'Logging in...' : 'Login'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
```

**Commit**: `git add web/src/app/login/page.tsx && git commit -m "feat: add login page"`

---

### Task 6.3: Create Auth Guard

**Files**: `web/src/components/AuthGuard.tsx`

```typescript
export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { isLoading, requiresLogin } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && requiresLogin) {
      router.push('/login');
    }
  }, [isLoading, requiresLogin, router]);

  if (isLoading) return <FullPageSpinner />;
  if (requiresLogin) return null;

  return <>{children}</>;
}
```

**Commit**: `git add web/src/components/AuthGuard.tsx && git commit -m "feat: add AuthGuard component"`

---

### Task 6.4: Add Error Boundary

**Files**: `web/src/components/ErrorBoundary.tsx`

```typescript
export class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { hasError: boolean; error?: Error }
> {
  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen flex items-center justify-center">
          <Card className="max-w-md">
            <CardHeader>
              <CardTitle className="text-destructive">Something went wrong</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground">{this.state.error?.message}</p>
            </CardContent>
            <CardFooter>
              <Button onClick={() => window.location.reload()}>Reload</Button>
            </CardFooter>
          </Card>
        </div>
      );
    }
    return this.props.children;
  }
}
```

**Commit**: `git add web/src/components/ErrorBoundary.tsx && git commit -m "feat: add error boundary"`

---

### Task 6.5: Update Root Layout

**Files**: `web/src/app/layout.tsx`

```typescript
export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="dark">
      <body>
        <Providers>
          <ErrorBoundary>
            <AuthGuard>
              {children}
            </AuthGuard>
          </ErrorBoundary>
        </Providers>
      </body>
    </html>
  );
}
```

**Commit**: `git add web/src/app/layout.tsx && git commit -m "feat: integrate auth guard and error boundary"`

---

### Task 6.6: Add Loading States

**Files**: Create skeleton components for all pages

```typescript
// web/src/components/ui/PageSkeleton.tsx
export function PageSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-48" />
      <div className="grid grid-cols-4 gap-4">
        {Array(4).fill(0).map((_, i) => (
          <Skeleton key={i} className="h-32" />
        ))}
      </div>
    </div>
  );
}
```

**Commit**: `git add web/src/components/ui/PageSkeleton.tsx && git commit -m "feat: add page skeleton component"`

---

### Task 6.7: Add Toast Notifications

**Files**: Already installed in Phase 0, integrate across pages

Update all mutations to show toast notifications (already done in previous phases).

---

### Task 6.8: Mobile Responsiveness Check

**Files**: Review all pages

Ensure all pages work on mobile:
- Stack columns on small screens
- Touch-friendly buttons (min 44px)
- Readable text sizes

**Commit**: `git commit -m "fix: mobile responsiveness improvements"`

---

### Task 6.9: Final Build Test

**Run**: `make build`
**Verify**: No errors, binary created in `bin/`

**Commit**: `git commit -m "chore: final build verification"`

---

## Phase 6 Complete

**Summary**: Auth & Polish with:
- ✅ Optional password protection
- ✅ Login/logout flow
- ✅ Error boundaries
- ✅ Loading states
- ✅ Mobile responsive
- ✅ Production ready

**Next**: [Phase 7: Testing & CI](./2026-01-31-web-dashboard-phase-7-testing.md)
