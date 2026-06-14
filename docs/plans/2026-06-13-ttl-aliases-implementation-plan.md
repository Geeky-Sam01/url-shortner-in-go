# Implementation Plan: Link Expiration (TTL) & Custom Aliases

We will add support for optional Custom Aliases (3-15 chars, alphanumeric/dashes/underscores) and Time-to-Live (TTL) expiration (1 hour, 1 day, 7 days, 30 days, or Never) on shortened links. 

When a link expires, visits will be redirected to a custom frontend `/expired` warnings page. Redis cache keys will be seeded with dynamic matching TTL duration for automatic cache clearance.

---

## User Review Required

> [!IMPORTANT]
> - Custom aliases will be validated to check for formatting and system route blacklist collisions (`dashboard`, `api`, `expired`, etc.) to prevent users from breaking application routing.
> - The database schema changes are backward-compatible and add a nullable `expires_at` column.

---

## Proposed Changes

### Go Backend (Database & Cache)

#### [MODIFY] [db.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/db/db.go)
Alter table to add `expires_at` column and its index for lookup optimization.
```go
	// Index on short_key for fast lookups during redirect.
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_urls_short_key ON urls(short_key);`)
	if err != nil {
		return fmt.Errorf("db: create idx_urls_short_key: %w", err)
	}

	// Add expires_at column to URLs table
	_, err = db.Exec(`ALTER TABLE urls ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE;`)
	if err != nil {
		return fmt.Errorf("db: add expires_at column: %w", err)
	}

	// Index on expires_at for fast checks
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls(expires_at);`)
	if err != nil {
		return fmt.Errorf("db: create idx_urls_expires_at: %w", err)
	}
```

#### [MODIFY] [url.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url.go)
1. Extend `ShortenRequest` to accept optional `alias` and `ttl_hours` fields.
2. In `CreateURL`, validate format/blacklist if `alias` is supplied, and check uniqueness. If `ttl_hours` is set, compute `expires_at` timestamp.
3. In `RedirectFallback`, read `expires_at` on cache miss. Redirect to `/expired?key=xyz` if current time is past expiration. Set matching remaining cache TTL.

```go
type ShortenRequest struct {
	URL      string `json:"url" binding:"required,url"`
	Alias    string `json:"alias" binding:"omitempty"`
	TTLHours int    `json:"ttl_hours" binding:"omitempty,min=0"`
}
```

```go
// Replace from line 84 to 154 inside CreateURL:
	var req ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or missing URL"})
		return
	}

	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL must start with http:// or https://"})
		return
	}

	// Prevent self-referential URL shortening
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL format"})
		return
	}
	inputHost := strings.ToLower(parsedURL.Host)
	if inputHost == strings.ToLower(c.Request.Host) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot shorten URLs pointing to this service"})
		return
	}
	if h.FrontendURL != "" {
		if parsedFrontend, err := url.Parse(h.FrontendURL); err == nil {
			if inputHost == strings.ToLower(parsedFrontend.Host) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot shorten URLs pointing to this service"})
				return
			}
		}
	}

	var shortKey string
	var expiresAt *time.Time

	if req.TTLHours > 0 {
		exp := time.Now().UTC().Add(time.Duration(req.TTLHours) * time.Hour)
		expiresAt = &exp
	}

	if req.Alias != "" {
		alias := strings.TrimSpace(req.Alias)
		// Validate alias format
		if len(alias) < 3 || len(alias) > 15 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Alias must be between 3 and 15 characters long"})
			return
		}
		for _, r := range alias {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Alias can only contain alphanumeric characters, dashes, and underscores"})
				return
			}
		}

		// Check reserved words
		blacklist := map[string]bool{
			"api": true, "health": true, "dashboard": true, "analytics": true,
			"architecture": true, "swagger": true, "expired": true,
		}
		if blacklist[strings.ToLower(alias)] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Alias is a reserved system route name"})
			return
		}

		// Check uniqueness
		var exists bool
		err = h.DB.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM urls WHERE short_key = $1)`, alias).Scan(&exists)
		if err != nil {
			slog.Error("failed to check alias uniqueness", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "Alias is already taken"})
			return
		}

		shortKey = alias

		_, err = h.DB.ExecContext(c.Request.Context(), `
			INSERT INTO urls (short_key, long_url, expires_at) 
			VALUES ($1, $2, $3)`, shortKey, req.URL, expiresAt)
		if err != nil {
			slog.Error("failed to insert url with alias", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save URL"})
			return
		}
	} else {
		var nextID int64
		err = h.DB.QueryRowContext(c.Request.Context(), `SELECT nextval('urls_id_seq')`).Scan(&nextID)
		if err != nil {
			slog.Error("failed to get next sequence value", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate ID"})
			return
		}

		shortKey = utils.Encode(nextID)

		_, err = h.DB.ExecContext(c.Request.Context(), `
			INSERT INTO urls (id, short_key, long_url, expires_at) 
			VALUES ($1, $2, $3, $4)`, nextID, shortKey, req.URL, expiresAt)
		if err != nil {
			slog.Error("failed to insert url", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save URL"})
			return
		}
	}

	// Cache the URL
	cacheTTL := 24 * time.Hour
	if expiresAt != nil {
		cacheTTL = expiresAt.Sub(time.Now().UTC())
	}
	if cacheTTL > 0 {
		if err := myredis.SetURLCache(c.Request.Context(), h.Redis, shortKey, req.URL, cacheTTL); err != nil {
			slog.Warn("failed to cache URL in redis", "error", err)
		}
	}

	c.JSON(http.StatusOK, ShortenResponse{
		ShortKey: shortKey,
		ShortURL: h.FrontendURL + "/" + shortKey,
	})
```

```go
// Replace from line 175 to 194 in RedirectFallback:
	// 2. Cache miss -> query DB
	if longURL == "" {
		var id int64
		var expiresAt sql.NullTime
		err := h.DB.QueryRowContext(c.Request.Context(), `
			SELECT id, long_url, expires_at FROM urls WHERE short_key = $1`, key).Scan(&id, &longURL, &expiresAt)
		
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
				return
			}
			slog.Error("db query error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		// Check if expired
		if expiresAt.Valid && time.Now().UTC().After(expiresAt.Time) {
			c.Redirect(http.StatusFound, h.FrontendURL + "/expired?key=" + key)
			return
		}

		// Update cache with remaining TTL
		cacheTTL := 24 * time.Hour
		if expiresAt.Valid {
			cacheTTL = expiresAt.Time.Sub(time.Now().UTC())
		}
		if cacheTTL > 0 {
			myredis.SetURLCache(c.Request.Context(), h.Redis, key, longURL, cacheTTL)
		}
	}
```

---

### Angular Frontend (Services, UI inputs, and warnings page)

#### [MODIFY] [api.service.ts](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/services/api.service.ts)
Update payload parameters in `shortenUrl`.
```typescript
  shortenUrl(longUrl: string, alias?: string, ttlHours?: number): Observable<ShortenResponse> {
    const payload: any = { url: longUrl };
    if (alias) payload.alias = alias;
    if (ttlHours) payload.ttl_hours = ttlHours;
    return this.http.post<ShortenResponse>(`${this.apiUrl}/shorten`, payload);
  }
```

#### [MODIFY] [dashboard.component.ts](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/dashboard/dashboard.component.ts)
Add signals and update `shorten()` helper to pass optional fields.
```typescript
  customAlias = signal('');
  selectedTTL = signal(0);
```
```typescript
  shorten() {
    const urlVal = this.longUrl();
    if (!urlVal) return;
    this.loading.set(true);
    this.api.shortenUrl(urlVal, this.customAlias(), this.selectedTTL()).subscribe({
      next: (res) => {
        this.longUrl.set('');
        this.customAlias.set('');
        this.selectedTTL.set(0);
        this.latestShortenedUrl.set(res.short_url);
        this.loadUrls();
        this.snackBar.open(`Shortened to: ${res.short_url}`, 'Close', { duration: 5000 });
      },
      error: (err) => {
        const errorMsg = err.error?.error || 'Error creating short URL';
        this.snackBar.open(errorMsg, 'Close', { duration: 4000 });
        console.error(err);
      },
      complete: () => this.loading.set(false)
    });
  }
```

#### [MODIFY] [dashboard.component.html](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/dashboard/dashboard.component.html)
Add settings expansion under the input console.
```html
  <!-- Input Wrapper -->
  <div class="input-console-wrapper">
    <div class="input-console" [class.focused]="isFocused()">
      <mat-icon class="console-icon">link</mat-icon>
      <input 
        class="console-input" 
        [ngModel]="longUrl()"
        (ngModelChange)="longUrl.set($event)"
        (focus)="isFocused.set(true)"
        (blur)="isFocused.set(false)"
        (keyup.enter)="shorten()"
        placeholder="Paste your long URL here..." 
      />
      <button 
        class="console-btn" 
        [disabled]="!longUrl() || loading()" 
        [class.shimmer-loading]="loading()"
        (click)="shorten()">
        <span class="btn-text">{{ loading() ? 'Generating...' : 'Shorten Link' }}</span>
        <mat-icon class="btn-icon">auto_awesome</mat-icon>
      </button>
    </div>

    <!-- Extra Options Drawer -->
    <div class="advanced-options">
      <div class="option-field">
        <mat-icon class="field-icon">vpn_key</mat-icon>
        <input 
          type="text" 
          [ngModel]="customAlias()" 
          (ngModelChange)="customAlias.set($event)"
          placeholder="Custom alias (optional, e.g. promo)" 
          class="field-input"
        />
      </div>
      <div class="option-field">
        <mat-icon class="field-icon">schedule</mat-icon>
        <select 
          [ngModel]="selectedTTL()" 
          (ngModelChange)="selectedTTL.set(Number($event))" 
          class="field-select">
          <option [value]="0">Link Expiration: Never</option>
          <option [value]="1">1 Hour</option>
          <option [value]="24">1 Day</option>
          <option [value]="168">7 Days</option>
          <option [value]="720">30 Days</option>
        </select>
      </div>
    </div>
  </div>
```

#### [MODIFY] [dashboard.component.css](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/dashboard/dashboard.component.css)
Add styles for advanced option boxes under the input console.
```css
/* Advanced Options Drawer */
.advanced-options {
  display: flex;
  gap: 16px;
  padding: 12px 8px 6px 8px;
  border-top: 1px solid var(--border-color);
  margin-top: 6px;
}

.option-field {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f8fafc;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 6px 12px;
}

.field-icon {
  font-size: 18px;
  width: 18px;
  height: 18px;
  color: var(--text-secondary);
}

.field-input {
  border: none;
  background: transparent;
  font-size: 13px;
  outline: none;
  color: var(--text-primary);
  width: 100%;
}

.field-select {
  border: none;
  background: transparent;
  font-size: 13px;
  outline: none;
  color: var(--text-primary);
  width: 100%;
  cursor: pointer;
  font-family: var(--font-family);
}

@media (max-width: 576px) {
  .advanced-options {
    flex-direction: column;
    gap: 8px;
  }
}
```

#### [NEW] [expired.component.ts](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/expired/expired.component.ts)
Create Expired warning component.
```typescript
import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';

@Component({
  selector: 'app-expired',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    MatCardModule,
    MatIconModule
  ],
  templateUrl: './expired.component.html',
  styleUrls: ['./expired.component.css']
})
export class ExpiredComponent {}
```

#### [NEW] [expired.component.html](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/expired/expired.component.html)
Expired link error UI.
```html
<div class="expired-container">
  <mat-card class="bento-card expired-card">
    <mat-icon class="expired-icon">lock_clock</mat-icon>
    <h1 class="heading-xl">Link Expired</h1>
    <p class="desc-para">
      The shortened link you are trying to access has reached its expiration date or time-to-live limit and is no longer active.
    </p>
    <button routerLink="/" class="back-btn">Go to Dashboard</button>
  </mat-card>
</div>
```

#### [NEW] [expired.component.css](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/expired/expired.component.css)
Styles for Expired Component.
```css
.expired-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 60vh;
  padding: 24px;
}

.expired-card {
  max-width: 480px;
  text-align: center;
  padding: 48px 32px;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.expired-icon {
  font-size: 64px;
  width: 64px;
  height: 64px;
  color: #ef4444;
  margin-bottom: 24px;
}

.heading-xl {
  font-size: 2.2rem;
  font-weight: 800;
  letter-spacing: -1px;
  color: var(--text-primary);
  margin-bottom: 16px;
}

.desc-para {
  font-size: 15px;
  line-height: 1.6;
  color: var(--text-secondary);
  margin-bottom: 32px;
}

.back-btn {
  background: var(--text-primary);
  border: none;
  border-radius: var(--radius-md);
  color: white;
  padding: 12px 24px;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.2s;
  text-decoration: none;
}

.back-btn:hover {
  background-color: #27272a;
}
```

#### [MODIFY] [app.routes.ts](file:///c:/Users/admin/Desktop/Projects/url-shortner/frontend/src/app/app.routes.ts)
Register `/expired` path.
```typescript
import { Routes } from '@angular/router';
import { DashboardComponent } from './dashboard/dashboard.component';
import { AnalyticsComponent } from './analytics/analytics.component';
import { ArchitectureComponent } from './architecture/architecture.component';
import { ExpiredComponent } from './expired/expired.component';

export const routes: Routes = [
  { path: '', component: DashboardComponent },
  { path: 'analytics', component: AnalyticsComponent },
  { path: 'architecture', component: ArchitectureComponent },
  { path: 'expired', component: ExpiredComponent },
  { path: '**', redirectTo: '' }
];
```

---

## Verification Plan

### Automated Verification
1. Run backend unit tests to ensure existing flows compile cleanly:
   `go test ./...` in `backend/` directory.
2. Verify frontend compilation:
   `npm run build` in `frontend/` directory.

### Manual Verification
1. Open the dashboard, type `https://google.com` as the URL.
2. In the advanced panel, set Custom Alias to `goog-alias` and expiration to `1 Hour`. Click "Shorten Link".
3. Check PostgreSQL database and verify that a row with `short_key = 'goog-alias'` and `expires_at` (populated with a time 1 hour in the future) was successfully inserted.
4. Verify that you can resolve the link `http://localhost:8080/goog-alias` and get redirected.
5. Create a link with alias `exp-alias` and set expiration to `1 Hour`. Then manually update `expires_at` in the DB to `time.Now() - 5 minutes`.
6. Visit `http://localhost:8080/exp-alias` in the browser and confirm it redirects to the frontend `/expired` route.
