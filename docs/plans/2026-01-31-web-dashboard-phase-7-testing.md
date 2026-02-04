# Phase 7: Testing & CI - JellyWatch Web Dashboard

> **Parent Plan**: [Web Dashboard Foundation](../2026-01-31-web-dashboard.md)
> **Previous**: [Phase 6: Auth & Polish](./2026-01-31-web-dashboard-phase-6-auth.md)

**Goal**: Comprehensive testing suite and CI/CD pipeline.

---

## Phase 7 Tasks

### Task 7.1: Setup Vitest for Unit Tests

**Files**: `web/package.json`, `web/vitest.config.ts`

```json
// Add to devDependencies
"@testing-library/react": "^14.0.0",
"@testing-library/jest-dom": "^6.0.0",
"vitest": "^1.0.0",
"jsdom": "^24.0.0",
"msw": "^2.0.0"
```

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts',
  },
});
```

**Commit**: `git add web/vitest.config.ts && git commit -m "test: setup vitest for unit testing"`

---

### Task 7.2: Create Test Utilities

**Files**: `web/src/test/setup.ts`, `web/src/test/mocks/handlers.ts`

```typescript
// web/src/test/setup.ts
import '@testing-library/jest-dom';
import { server } from './mocks/server';

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```

**Commit**: `git add web/src/test/ && git commit -m "test: add test utilities and MSW setup"`

---

### Task 7.3: Write Component Tests

**Files**: `web/src/components/**/*.test.tsx`

```typescript
// web/src/components/dashboard/StatCard.test.tsx
import { render, screen } from '@testing-library/react';
import { StatCard } from './StatCard';
import { Database } from 'lucide-react';

test('renders stat card with value', () => {
  render(<StatCard title="Files" value="1,234" icon={Database} />);
  expect(screen.getByText('Files')).toBeInTheDocument();
  expect(screen.getByText('1,234')).toBeInTheDocument();
});
```

**Commit**: `git add web/src/components/**/*.test.tsx && git commit -m "test: add component unit tests"`

---

### Task 7.4: Setup Playwright for E2E

**Files**: `web/playwright.config.ts`

```typescript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:8686',
    trace: 'on-first-retry',
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
});
```

**Commit**: `git add web/playwright.config.ts && git commit -m "test: setup playwright for E2E testing"`

---

### Task 7.5: Write E2E Tests

**Files**: `web/e2e/*.spec.ts`

Already created for Dashboard and Duplicates in previous phases.

Add tests for:
- Authentication flow
- Queue operations
- Navigation between pages

**Commit**: `git add web/e2e/ && git commit -m "test: add comprehensive E2E tests"`

---

### Task 7.6: Setup GitHub Actions CI

**Files**: `.github/workflows/ci.yml`

Already created in Phase 0. Verify it includes:
- Go tests
- Frontend type check
- Frontend build
- E2E tests (optional, can be heavy)

**Commit**: `git add .github/workflows/ && git commit -m "ci: add GitHub Actions workflow"`

---

### Task 7.7: Add Test Scripts to Makefile

**Files**: `Makefile`

```makefile
test: test-go test-web

test-go:
	go test -v ./...

test-web:
	cd web && npm test

test-e2e:
	cd web && npx playwright test
```

**Commit**: `git add Makefile && git commit -m "build: add test targets to Makefile"`

---

### Task 7.8: Coverage Reporting

**Files**: `web/vitest.config.ts`

```typescript
export default defineConfig({
  test: {
    coverage: {
      reporter: ['text', 'json', 'html'],
      exclude: ['node_modules/', 'src/test/'],
    },
  },
});
```

**Commit**: `git add web/vitest.config.ts && git commit -m "test: add coverage reporting"`

---

## Phase 7 Complete

**Summary**: Testing & CI with:
- ✅ Vitest for unit tests
- ✅ React Testing Library
- ✅ MSW for API mocking
- ✅ Playwright for E2E
- ✅ GitHub Actions CI
- ✅ Coverage reporting

**Next**: [Phase 8: Final Integration](./2026-01-31-web-dashboard-phase-8-final.md)
