# Design Document: Frontend UI Refactor & Zoneless State Fix

This document outlines the technical design for refactoring the URL shortener frontend. The changes aim to resolve state rendering issues caused by Angular's Zoneless Change Detection and realign the user interface to match the clean, premium SaaS references.

## Core Goals

1. **State Reliability (Signals):** Resolve the bug where "Creating..." remains stuck and the URL table fails to update. We will migrate component state variables to Angular Signals, guaranteeing immediate DOM updates when asynchronous HTTP callbacks complete.
2. **Visual Re-alignment (Light Theme SaaS):** Match the reference designs (Shorten.io and Lumina) with a clean, high-density light-mode layout.
3. **Pill Component:** Instantly render a shortened URL pill with copy and navigate actions right under the input field.
4. **Shimmer Animation:** Implement a smooth shimmery "Generating..." state on the button when request is in progress.
5. **Mobile-specific Reordering:** Display "Recent Links" above "Insights/Overview" on mobile viewport sizes, while keeping the standard two-column layout on desktop.

---

## Technical Architecture

### 1. State Management (Signals)
We will refactor `AppComponent` to use standard Angular Signals:
- `longUrl = signal('')`
- `loading = signal(false)`
- `isFocused = signal(false)`
- `recentUrls = signal<UrlItem[]>(交互)`
- `latestShortenedUrl = signal<string | null>(null)`

By updating these values via `.set()`, Angular's Zoneless engine is notified of the dirty components, scheduling rendering ticks automatically.

### 2. Styling Strategy
Instead of fighting heavy Angular Material component overrides, we will use basic HTML semantic tags customized with standard CSS. We will keep `mat-icon` for vector icons.
- **Colors:** Dominant clean whites (`#ffffff`) and off-whites (`#f8fafc`), dark slate text (`#0f172a`), slate-indigo highlights (`#4f46e5`), and subtle borders (`#e2e8f0`).
- **Typography:** Outfit / Inter.

### 3. Responsive Layout (Bento Grid)
We will leverage CSS Grid for desktop columns and Flexbox for mobile:
```css
.bento-grid {
  display: grid;
  grid-template-columns: 3fr 2fr;
  gap: 32px;
}

@media (max-width: 768px) {
  .bento-grid {
    display: flex;
    flex-direction: column;
  }
  .left-card {
    order: 1; /* Recent Links */
  }
  .right-card {
    order: 2; /* Insights/Overview */
  }
}
```

---

## Proposed Layout Details

### Navbar
- Brand title: `Shorten.io`
- Centered links: `Dashboard`, `Analytics`, `Campaigns`, `Settings` (no "Create New" or profile photo, as per instructions).

### Input Box & Shimmer State
- Input field with standard input and a modern action button.
- Button text changes from `Shorten Link` to `Generating...` with a shimmering keyframe animation when `loading` signal is true.

### Result Pill (Under Input Area)
A dynamic container shown only when `latestShortenedUrl()` is populated:
```html
<div class="result-pill" *ngIf="latestShortenedUrl()">
  <span class="url-text">{{ latestShortenedUrl() }}</span>
  <div class="pill-actions">
    <button (click)="copyToClipboard(latestShortenedUrl()!)" title="Copy"><mat-icon>content_copy</mat-icon></button>
    <button (click)="navigateToUrl(latestShortenedUrl()!)" title="Visit"><mat-icon>open_in_new</mat-icon></button>
  </div>
</div>
```

---

## Verification Plan

### Manual Verification
1. Verify that shortening a link changes button text to a shimmering "Generating...", then resets instantly.
2. Confirm the short link result pill appears below the input area immediately on response.
3. Check that the "Recent Links" table gets updated instantly with the newly created URL.
4. Verify that copy and visit links function correctly inside the pill.
5. Inspect the mobile viewport (under 768px wide) and confirm the "Recent Links" block is displayed above the "Insights" block.
