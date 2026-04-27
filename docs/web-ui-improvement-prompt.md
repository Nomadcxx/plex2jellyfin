# Web UI Systematic Improvement - Agent Prompt

## Project Context

**Jellywatch** is a media file organizer for Jellyfin. It has a Next.js 14 web UI that needs systematic visual improvement.

### Tech Stack
- **Framework**: Next.js 14 (App Router, static export)
- **Styling**: Tailwind CSS + shadcn/ui (Radix primitives)
- **State**: TanStack Query v5 + Zustand
- **Icons**: Lucide React
- **Theme**: Dark mode (zinc-950 background, zinc-100 text)

### Available UI Components (web/src/components/ui/)
- Dialog, Popover, Progress, ScrollArea, Separator, Tabs, Tooltip
- Custom: Button, Card patterns using Radix + CVA

### Pages to Improve
| Route | File | Purpose |
|-------|------|---------|
| `/` | `app/page.tsx` | Dashboard - stats cards, media managers |
| `/onboarding` | `app/onboarding/page.tsx` | New user wizard (4 steps) |
| `/duplicates` | `app/duplicates/page.tsx` | Duplicate file management |
| `/consolidation` | `app/consolidation/page.tsx` | Scattered series consolidation |
| `/queue` | `app/queue/page.tsx` | Sonarr/Radarr queue management |
| `/activity` | `app/activity/page.tsx` | Activity log with SSE updates |
| `/settings` | `app/settings/page.tsx` | Settings display |
| `/login` | `app/login/page.tsx` | Authentication |

### Layout Components
- `components/layout/AppShell.tsx` - Main layout wrapper with sidebar
- `components/layout/Sidebar.tsx` - Navigation sidebar

---

## Your Mission

Systematically improve the visual design and UX of this web UI. The current UI is functional but visually basic - it needs to feel polished, modern, and delightful to use.

---

## Design Principles to Follow

### 1. Visual Hierarchy
- Clear distinction between primary, secondary, and tertiary content
- Proper use of whitespace (breathing room)
- Consistent typography scale
- Strategic use of color for emphasis

### 2. Micro-interactions
- Hover states that provide feedback
- Smooth transitions (150-300ms)
- Loading states that feel intentional
- Success/error feedback animations

### 3. Card Design
- Subtle shadows or borders for depth
- Consistent padding and border-radius
- Group related information visually
- Consider glassmorphism for elevated elements

### 4. Color Usage
- Current: zinc-950 bg, zinc-900 cards, zinc-800 borders
- Add accent color for actions (recommend: violet, blue, or emerald)
- Status colors: green (success), amber (warning), red (error)
- Gradients for hero elements or emphasis

### 5. Typography
- Clear heading hierarchy (text-3xl → text-xl → text-lg → text-base)
- Muted text for secondary info (text-zinc-400)
- Font weights: bold for headings, medium for labels, normal for body

### 6. Icons
- Consistent sizing (h-4 w-4 for inline, h-5 w-5 for buttons, h-6+ for features)
- Muted color (text-zinc-500) unless actionable
- Consider filled vs outlined based on state

---

## Specific Improvements Needed

### Dashboard (`/`)
- [ ] Add visual interest to stat cards (gradients, icons with color)
- [ ] Media manager cards need status indicator styling
- [ ] Consider a hero section or welcome message
- [ ] Add quick action buttons

### Onboarding (`/onboarding`)
- [ ] Step indicator needs better visual design (not just circles)
- [ ] Add illustrations or icons for each step
- [ ] Progress animation between steps
- [ ] More engaging "Complete" celebration

### Duplicates (`/duplicates`)
- [ ] File comparison view needs clear visual diff
- [ ] Quality badges (REMUX, BluRay, WEB-DL) with distinct styling
- [ ] Action buttons need clearer hierarchy
- [ ] Empty state design

### Consolidation (`/consolidation`)
- [ ] Series cards with poster/thumbnail placeholder
- [ ] Episode count badges
- [ ] Before/after path visualization
- [ ] Consolidation progress indicator

### Queue (`/queue`)
- [ ] Tab design for Sonarr/Radarr
- [ ] Queue item cards with progress
- [ ] Status badges (downloading, queued, stuck)
- [ ] Bulk action toolbar

### Activity (`/activity`)
- [ ] Timeline-style layout for activities
- [ ] Activity type icons with color coding
- [ ] Relative timestamps ("2 min ago")
- [ ] Live update indicator (SSE pulse)

### Settings (`/settings`)
- [ ] Section grouping with headers
- [ ] Toggle/switch components for booleans
- [ ] Input fields with proper styling
- [ ] Save confirmation

### Sidebar/Navigation
- [ ] Active state styling
- [ ] Hover effects
- [ ] Icon + text alignment
- [ ] Collapse/expand animation
- [ ] Logo/brand area

---

## Implementation Approach

### Phase 1: Foundation
1. Update `globals.css` with CSS variables for accent colors
2. Create reusable styled components (Badge, StatusDot, Card variants)
3. Establish consistent animation classes

### Phase 2: Layout Polish
4. Improve AppShell and Sidebar
5. Add proper loading skeletons
6. Implement toast notifications (sonner is installed)

### Phase 3: Page-by-Page
7. Dashboard improvements
8. Onboarding flow polish
9. Duplicates page
10. Consolidation page
11. Queue page
12. Activity page
13. Settings page
14. Login page

### Phase 4: Final Polish
15. Empty states for all pages
16. Error states
17. Mobile responsiveness check
18. Animation consistency pass

---

## Constraints

### MUST DO
- Preserve all existing functionality
- Use only existing dependencies (no new npm packages)
- Maintain TypeScript type safety
- Keep dark theme as default
- Test that pages still render after changes

### MUST NOT DO
- Do not change API calls or hooks logic
- Do not modify the Go backend
- Do not change routing structure
- Do not remove existing features
- Do not add heavy animations that impact performance

### Verification After Each Change
```bash
cd /home/nomadx/Documents/jellywatch/web
npm run build
# Should complete without errors
```

---

## File Locations

```
web/src/
├── app/
│   ├── globals.css          # Global styles, CSS variables
│   ├── layout.tsx           # Root layout
│   ├── page.tsx             # Dashboard
│   ├── onboarding/page.tsx
│   ├── duplicates/page.tsx
│   ├── consolidation/page.tsx
│   ├── queue/page.tsx
│   ├── activity/page.tsx
│   ├── settings/page.tsx
│   └── login/page.tsx
├── components/
│   ├── layout/
│   │   ├── AppShell.tsx
│   │   └── Sidebar.tsx
│   └── ui/                  # shadcn/ui components
├── hooks/                   # Data fetching hooks
└── lib/
    └── utils.ts             # cn() utility for classnames
```

---

## Current Design Tokens

```css
/* From globals.css */
:root {
  --background: 240 10% 3.9%;
  --foreground: 0 0% 98%;
}

/* Common Tailwind classes used */
bg-zinc-950    /* Page background */
bg-zinc-900    /* Card background */
border-zinc-800 /* Borders */
text-zinc-100  /* Primary text */
text-zinc-400  /* Muted text */
text-zinc-600  /* Very muted (icons) */
```

---

## Example: Improved Stat Card

**Before:**
```tsx
<div className="bg-zinc-900 p-6 rounded-lg border border-zinc-800">
  <p className="text-sm text-zinc-400">{title}</p>
  <p className="text-2xl font-bold mt-1">{value}</p>
</div>
```

**After:**
```tsx
<div className="group relative bg-zinc-900/50 p-6 rounded-xl border border-zinc-800 
                hover:border-zinc-700 hover:bg-zinc-900 transition-all duration-200
                backdrop-blur-sm">
  <div className="absolute inset-0 bg-gradient-to-br from-violet-500/5 to-transparent 
                  rounded-xl opacity-0 group-hover:opacity-100 transition-opacity" />
  <div className="relative flex items-center justify-between">
    <div>
      <p className="text-sm font-medium text-zinc-400">{title}</p>
      <p className="text-3xl font-bold mt-2 bg-gradient-to-r from-white to-zinc-400 
                    bg-clip-text text-transparent">{value}</p>
    </div>
    <div className="p-3 rounded-xl bg-violet-500/10">
      <Icon className="h-6 w-6 text-violet-400" />
    </div>
  </div>
</div>
```

---

## Start Here

1. Read `web/src/app/globals.css` and `web/tailwind.config.js`
2. Review `web/src/components/layout/AppShell.tsx` and `Sidebar.tsx`
3. Start with Dashboard (`web/src/app/page.tsx`) as the reference design
4. Apply consistent patterns to other pages

Good luck! Make it beautiful. 🎨
