# Implementation Plan: Architecture Tab with Stepper & Inline SVGs

We will add a new "Architecture" tab to the URL Shortener application to explain the high-performance system design, scalability features (Redis Queue buffering, Vercel Edge caching, and asynchronous analytics worker), and database design. 

We will use pre-rendered responsive inline SVGs inside a Material Stepper with cards to ensure instant, ultra-fast load times.

---

## User Review Required

> [!NOTE]
> Since we decided to use Approach 2 (Pre-rendered inline SVGs), there is zero runtime dependency on dynamic Mermaid compilers or external CDN fetches, ensuring instant load time and absolute safety in sandbox/offline environments.

---

## Proposed Changes

### Navigation & Routing Setup

#### [MODIFY] [app.component.html](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/app.component.html)
Add the "Architecture" navigation link after "Analytics".
```html
    <nav class="navbar-links">
      <a routerLink="/" routerLinkActive="active" [routerLinkActiveOptions]="{exact: true}" class="nav-link">Dashboard</a>
      <a routerLink="/analytics" routerLinkActive="active" class="nav-link">Analytics</a>
      <a routerLink="/architecture" routerLinkActive="active" class="nav-link">Architecture</a>
    </nav>
```

#### [MODIFY] [app.routes.ts](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/app.routes.ts)
Register `/architecture` route mapping to the new `ArchitectureComponent`.
```typescript
import { Routes } from '@angular/router';
import { DashboardComponent } from './dashboard/dashboard.component';
import { AnalyticsComponent } from './analytics/analytics.component';
import { ArchitectureComponent } from './architecture/architecture.component';

export const routes: Routes = [
  { path: '', component: DashboardComponent },
  { path: 'analytics', component: AnalyticsComponent },
  { path: 'architecture', component: ArchitectureComponent },
  { path: '**', redirectTo: '' }
];
```

---

### Architecture Component

#### [NEW] [architecture.component.ts](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/architecture/architecture.component.ts)
Create the standalone Angular component, importing necessary Material modules.
```typescript
import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';
import { MatStepperModule } from '@angular/material/stepper';
import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';

@Component({
  selector: 'app-architecture',
  standalone: true,
  imports: [
    CommonModule,
    MatStepperModule,
    MatCardModule,
    MatIconModule
  ],
  templateUrl: './architecture.component.html',
  styleUrls: ['./architecture.component.css']
})
export class ArchitectureComponent {}
```

#### [NEW] [architecture.component.html](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/architecture/architecture.component.html)
Create the template housing the stepper, descriptive text, and inline SVGs.
```html
<div class="architecture-header">
  <h1 class="heading-xl">System Architecture</h1>
  <p class="subtitle">Understand how our high-performance url-shortener scales and routes globally.</p>
</div>

<div class="stepper-container">
  <mat-stepper orientation="vertical" [linear]="false">
    
    <!-- Step 1: Edge Redirection -->
    <mat-step label="Edge Redirection Flow (Sub-50ms Global Routing)">
      <mat-card class="bento-card arch-card">
        <div class="card-content">
          <p class="desc-para">
            When a user visits a short link like <code>short.en/xyz</code>, the request is intercepted directly at <strong>Vercel's Edge Nodes</strong> close to their physical location.
          </p>
          <ul class="scale-points">
            <li><strong>Scale Edge Routing:</strong> Edge middleware runs globally in light V8 sandboxes, avoiding cold boots and network roundtrips back to primary servers.</li>
            <li><strong>O(1) Redis Caching:</strong> Fast O(1) reads query the Upstash Redis replica globally. If hit, client redirects immediately.</li>
            <li><strong>Fail-safe Fallback:</strong> If a cache miss occurs, the Edge middleware transparently forwards the request to the Go backend API to fetch from Postgres and cache it.</li>
          </ul>

          <div class="svg-container">
            <svg viewBox="0 0 800 240" class="arch-svg" width="100%">
              <defs>
                <marker id="arrow" viewBox="0 0 10 10" refX="6" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                  <path d="M 0 0 L 10 5 L 0 10 z" fill="#1A1A1A"/>
                </marker>
              </defs>
              <!-- Nodes -->
              <rect x="20" y="90" width="120" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="80" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">User Client</text>

              <rect x="220" y="90" width="160" height="60" rx="8" fill="#F4F4F5" stroke="#1A1A1A" stroke-width="2"/>
              <text x="300" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Vercel Edge</text>

              <rect x="460" y="30" width="140" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="530" y="65" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Redis (Upstash)</text>

              <rect x="460" y="150" width="140" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="530" y="185" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Go Backend</text>

              <rect x="680" y="90" width="100" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="730" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Destination</text>

              <!-- Connectors -->
              <path d="M 140 120 L 210 120" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 380 110 L 450 70" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 380 130 L 450 170" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 600 60 L 670 110" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 600 180 L 670 140" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>

              <!-- Labels -->
              <text x="175" y="112" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">visits key</text>
              <text x="415" y="80" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">1. lookup</text>
              <text x="415" y="162" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">2. fallback</text>
            </svg>
          </div>
        </div>
      </mat-card>
    </mat-step>

    <!-- Step 2: URL Creation -->
    <mat-step label="Decoupled Write Path (URL Creation & Caching)">
      <mat-card class="bento-card arch-card">
        <div class="card-content">
          <p class="desc-para">
            When creating a shortened URL, the client calls our Go REST API backend. The write path is optimized for consistency while ensuring subsequent reads hit cache instantly.
          </p>
          <ul class="scale-points">
            <li><strong>Parallel Cache Seeding:</strong> Go writes to the persistent PostgreSQL DB and seeds the Redis cache concurrently, so the first redirection visit requires zero DB queries.</li>
            <li><strong>Golang Concurrency:</strong> Goroutines process non-blocking backend tasks while returning a rapid JSON payload containing the short key back to the client.</li>
          </ul>

          <div class="svg-container">
            <svg viewBox="0 0 800 240" class="arch-svg" width="100%">
              <defs>
                <marker id="arrow" viewBox="0 0 10 10" refX="6" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                  <path d="M 0 0 L 10 5 L 0 10 z" fill="#1A1A1A"/>
                </marker>
              </defs>
              <!-- Nodes -->
              <rect x="40" y="90" width="150" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="115" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Angular Client</text>

              <rect x="290" y="90" width="160" height="60" rx="8" fill="#F4F4F5" stroke="#1A1A1A" stroke-width="2"/>
              <text x="370" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Go Backend API</text>

              <rect x="550" y="30" width="160" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="630" y="65" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Redis (Write Cache)</text>

              <rect x="550" y="150" width="160" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="630" y="185" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">PostgreSQL DB</text>

              <!-- Connectors -->
              <path d="M 190 120 L 280 120" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 450 110 L 540 70" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 450 130 L 540 170" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>

              <!-- Labels -->
              <text x="235" y="112" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">POST /api/shorten</text>
              <text x="495" y="80" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">1. Set Cache</text>
              <text x="495" y="162" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">2. Save URL</text>
            </svg>
          </div>
        </div>
      </mat-card>
    </mat-step>

    <!-- Step 3: Asynchronous Worker -->
    <mat-step label="Asynchronous Analytics Worker (Redis Queue Buffering)">
      <mat-card class="bento-card arch-card">
        <div class="card-content">
          <p class="desc-para">
            Traditional SQL databases choke when writing analytical logs (IPs, locations, dates) under heavy traffic spikes. We use an event-driven architecture to keep redirections sub-millisecond.
          </p>
          <ul class="scale-points">
            <li><strong>Redis Queue Buffering:</strong> Redirection events push click data straight to a fast Redis list (acting as an in-memory queue), bypassing DB writes on the user response path.</li>
            <li><strong>Batch flushing:</strong> A background Go worker pulls batches of events from Redis periodically and flushes them in bulk to Postgres.</li>
            <li><strong>Isolation:</strong> Heavy read analytics (e.g., loading the Dashboard charts) do not block or lag redirections, as the SQL persistent storage is decoupled via the worker.</li>
          </ul>

          <div class="svg-container">
            <svg viewBox="0 0 800 240" class="arch-svg" width="100%">
              <defs>
                <marker id="arrow" viewBox="0 0 10 10" refX="6" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                  <path d="M 0 0 L 10 5 L 0 10 z" fill="#1A1A1A"/>
                </marker>
              </defs>
              <!-- Nodes -->
              <rect x="20" y="90" width="150" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="95" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Redirection Event</text>

              <rect x="240" y="90" width="150" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="315" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Redis Queue</text>

              <rect x="460" y="90" width="160" height="60" rx="8" fill="#F4F4F5" stroke="#1A1A1A" stroke-width="2"/>
              <text x="540" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Analytics Worker</text>

              <rect x="680" y="90" width="100" height="60" rx="8" fill="#FFFFFF" stroke="#1A1A1A" stroke-width="2"/>
              <text x="730" y="125" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="bold" text-anchor="middle" fill="#1A1A1A">Postgres DB</text>

              <!-- Connectors -->
              <path d="M 170 120 L 230 120" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 390 120 L 450 120" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>
              <path d="M 620 120 L 670 120" stroke="#1A1A1A" stroke-width="2" marker-end="url(#arrow)"/>

              <!-- Labels -->
              <text x="200" y="112" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">1. Push</text>
              <text x="420" y="112" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">2. Batch Poll</text>
              <text x="645" y="112" font-family="Space Grotesk, sans-serif" font-size="11" text-anchor="middle" fill="#71717A">3. Save</text>
            </svg>
          </div>
        </div>
      </mat-card>
    </mat-step>
  </mat-stepper>
</div>
```

#### [NEW] [architecture.component.css](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/architecture/architecture.component.css)
Style rules for headers, description nodes, list elements, and SVGs inside the stepper cards.
```css
.architecture-header {
  text-align: center;
  padding: 40px 0;
}

.heading-xl {
  font-size: 3.5rem;
  font-weight: 800;
  letter-spacing: -1.5px;
  line-height: 1.1;
  color: var(--text-primary);
  margin-bottom: 16px;
}

.subtitle {
  color: var(--text-secondary);
  font-size: 15px;
  margin-top: 12px;
}

.stepper-container {
  max-width: 900px;
  margin: 0 auto;
  padding-bottom: 60px;
}

/* Material Stepper Overrides */
::ng-deep .mat-vertical-stepper-header {
  padding: 24px 8px !important;
}

::ng-deep .mat-step-label {
  font-family: var(--font-family) !important;
  font-size: 16px !important;
  font-weight: 700 !important;
  color: var(--text-primary) !important;
}

::ng-deep .mat-step-icon {
  background-color: var(--text-primary) !important;
  color: white !important;
}

/* Card layout */
.arch-card {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 24px;
  margin: 16px 0;
  background: var(--bg-card);
}

.desc-para {
  font-size: 15px;
  line-height: 1.6;
  color: var(--text-primary);
  margin-bottom: 16px;
}

.scale-points {
  padding-left: 20px;
  margin-bottom: 24px;
}

.scale-points li {
  margin-bottom: 8px;
  font-size: 14px;
  line-height: 1.5;
  color: var(--text-secondary);
}

.scale-points li strong {
  color: var(--text-primary);
}

/* SVG Rendering */
.svg-container {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 16px;
  background: #fafafa;
  display: flex;
  justify-content: center;
  align-items: center;
  margin-top: 16px;
}

.arch-svg {
  max-height: 240px;
  width: 100%;
}

@media (max-width: 768px) {
  .heading-xl {
    font-size: 2.5rem;
  }
  .arch-card {
    padding: 16px;
  }
}
```

---

## Verification Plan

### Automated Verification
Run compile check to confirm Angular compiles cleanly:
`npm run build` inside `frontend/` directory.

### Manual Verification
1. Open dashboard, verify that "Architecture" nav link is present next to "Analytics".
2. Click the link, confirm browser route navigates to `/architecture`.
3. Verify that the stepper steps render, and clicking any step expands the card details instantly.
4. Verify that the flow SVGs are drawn correctly and scale down responsively on a mobile viewport size without cropping.
