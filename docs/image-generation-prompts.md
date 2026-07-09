# Image Generation Prompts for Plex2Jellyfin UI

Use these prompts with DALL-E 3, Midjourney, or any AI image generator to create PNG illustrations.

## Style Guidelines
- **Theme:** Modern, minimal, dark-tech aesthetic
- **Colors:** Deep blacks (#09090b), zinc grays (#18181b), violet accents (#8b5cf6), emerald greens (#10b981)
- **Style:** Clean vector-like illustration, flat design with subtle gradients
- **Format:** PNG with transparent background (where applicable)
- **Size:** 512x512px or 1024x1024px

---

## Empty State Illustrations

### 1. empty-dashboard.png
**Prompt:**
```
A minimalist illustration of a media library dashboard interface on a dark background. Shows floating cards with media icons (film, tv), charts, and statistics in a modern glassmorphism style. Deep black background (#09090b) with violet (#8b5cf6) and emerald (#10b981) accent glows. Clean vector style, no text. Professional tech aesthetic.
```

### 2. empty-duplicates.png
**Prompt:**
```
A minimalist illustration showing file organization and cleanup. Two overlapping file icons merging into one, with a checkmark. Dark background (#09090b) with emerald green (#10b981) glow effects. Clean modern vector style, no text. Represents duplicate file removal and space optimization.
```

### 3. empty-consolidation.png
**Prompt:**
```
A minimalist illustration of scattered folder icons organizing themselves into a neat stack. Shows folders flying from multiple locations into one central organized structure. Dark background (#09090b) with sky blue (#0ea5e9) accent glows. Clean vector style, no text. Represents file consolidation and organization.
```

### 4. empty-queue.png
**Prompt:**
```
A minimalist illustration of download progress. Shows a download arrow pointing into a folder with a progress bar. Dark background (#09090b) with blue (#3b82f6) accent glows. Clean modern vector style, no text. Represents media downloading and queue management.
```

### 5. empty-activity.png
**Prompt:**
```
A minimalist illustration of a timeline or activity feed. Shows a vertical line with dots/bubbles representing events, with a gentle glow effect. Dark background (#09090b) with fuchsia (#d946ef) accent glows. Clean vector style, no text. Represents activity monitoring and event tracking.
```

---

## Onboarding Illustrations

### 6. onboarding-scan.png
**Prompt:**
```
A minimalist illustration of file scanning and analysis. Shows a magnifying glass scanning over folders with files, detecting media content. Rays of light or scanning lines emanating from the magnifier. Dark background (#09090b) with violet (#8b5cf6) and amber (#f59e0b) accent glows. Clean vector style, no text. Represents the scanning process.
```

### 7. onboarding-duplicates.png
**Prompt:**
```
A minimalist illustration of duplicate detection. Shows two identical file icons side by side with a comparison visualization between them. One file is highlighted/emphasized while the other fades. Dark background (#09090b) with violet (#8b5cf6) accent glows. Clean vector style, no text. Represents finding and comparing duplicate files.
```

### 8. onboarding-consolidate.png
**Prompt:**
```
A minimalist illustration of file consolidation. Shows scattered TV show folders from different locations flowing together into one organized folder structure with seasons. Arrows showing the movement. Dark background (#09090b) with sky blue (#0ea5e9) accent glows. Clean vector style, no text. Represents organizing scattered series.
```

### 9. onboarding-complete.png
**Prompt:**
```
A celebratory minimalist illustration showing a completed media library. A checkmark badge with sparkles/confetti, surrounded by organized folders and media icons. Success and completion vibes. Dark background (#09090b) with emerald green (#10b981) and gold accent glows. Clean vector style, no text. Represents successful setup completion.
```

---

## Output Specifications

Save all generated images to: `web/public/illustrations/`

| Filename | Usage Location |
|----------|---------------|
| empty-dashboard.png | Dashboard empty state |
| empty-duplicates.png | Duplicates page success state |
| empty-consolidation.png | Consolidation page success state |
| empty-queue.png | Queue page empty state |
| empty-activity.png | Activity page empty state |
| onboarding-scan.png | Onboarding Step 1 |
| onboarding-duplicates.png | Onboarding Step 2 |
| onboarding-consolidate.png | Onboarding Step 3 |
| onboarding-complete.png | Onboarding Step 4 |

---

## Recommended Tools

- **DALL-E 3** (ChatGPT Plus): Best for following exact color specifications
- **Midjourney v6**: Best artistic quality, may need color adjustment post-processing
- **Leonardo.ai**: Good free alternative with style controls
- **Adobe Firefly**: Good for commercial-safe outputs

## Post-Processing Tips

1. Remove any text that AI adds (the prompts specify "no text")
2. Adjust colors to match exact hex codes if needed
3. Ensure backgrounds are pure black (#09090b) or transparent
4. Optimize file size using tools like TinyPNG or Squoosh
