# Design Document: Architecture Tab with Pre-rendered Flowcharts

This document outlines the design and implementation details for adding the **Architecture** tab to the URL Shortener frontend application. 

## Goals
1. Provide a step-by-step walkthrough of the system's architecture using a Material Stepper.
2. Emphasize scalability, event buffering (Redis Queue), edge performance, and decoupling of read/write paths.
3. Deliver an optimal, ultra-fast load time using pre-rendered inline SVGs (monochrome, high-contrast style guide).

---

## Technical Details

### 1. Route Configuration
We will register a new route under `/architecture` mapping to a new component `ArchitectureComponent`:
* **Path:** `/architecture`
* **Route file:** `frontend/src/app/app.routes.ts`

### 2. UI Stepper Component
* **Package:** `@angular/material/stepper` (specifically `<mat-stepper>` and `<mat-step>`).
* **Design Theme:** Solid black/white card styling matching the Lumina/Shorten.io visual design system.
* **Layout:** A linear/non-linear horizontal/vertical stepper containing:
  - Markdown-styled text (paragraphs, bullet points, code tags) highlighting system design decisions and scalability features.
  - Inline high-contrast responsive SVG diagrams describing the flow visually.

### 3. Architecture Steps & Flows

#### Step 1: Edge Redirection Flow (Fast Path)
* **Description:** Edge-based request interception.
* **Scalability Feature:** Direct redirection at Vercel's Edge nodes close to the user worldwide, backed by an O(1) Redis read cache. Sub-50ms latency.
* **Visual Diagram:** Client request hitting Vercel Edge $\rightarrow$ Redis cache hit (instant redirect) or cache miss (fallback to Go API + DB).

#### Step 2: Write Path Decoupling (URL Creation)
* **Description:** Synchronous URL shortening.
* **Scalability Feature:** Decouples persistent writes from cached redirects. The short code is saved to PostgreSQL and concurrently cached in Redis to satisfy future redirects instantly.
* **Visual Diagram:** Angular Client POST $\rightarrow$ Go Gin REST API $\rightarrow$ parallel writes to PostgreSQL (durability) and Redis cache (speed).

#### Step 3: Asynchronous Analytics Worker (Redis Queue)
* **Description:** Asynchronous analytics logging.
* **Scalability Feature:** Protects primary database from write amplification under heavy traffic. Click events are buffered into a Redis Queue immediately upon redirection, then flushed in batches to Postgres by a Go Background Worker.
* **Visual Diagram:** Redirection click $\rightarrow$ Redis queue push $\rightarrow$ Go Background Worker polling/flushing in batches $\rightarrow$ PostgreSQL analytics tables.

---

## Visual styling of SVGs
- Stroke color: `#1A1A1A`
- Fill colors: `#FFFFFF` and `#F4F4F5` (subtle gray)
- Stroke-width: `2px`
- Font: `Space Grotesk, sans-serif` or `Inter, sans-serif`
- Arrows: SVG path markers using `#1A1A1A`
- Responsive behavior: `viewBox="0 0 800 200"` with `width="100%"` and max-height constraints.

---

## Verification Plan
1. Check that clicking the "Architecture" tab in the navbar successfully navigates to `/architecture`.
2. Inspect the stepper and verify all cards and descriptions load instantly without any delay.
3. Validate that SVG diagrams adapt and scale correctly on mobile viewports (<768px) and don't crop.
4. Build the production build (`npm run build`) to ensure all Angular components compile cleanly.
