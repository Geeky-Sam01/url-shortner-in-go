import { Component, inject, OnInit, signal, computed } from '@angular/core';
import { CommonModule, DOCUMENT } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ApiService, UrlItem } from '../services/api.service';
import { MatIconModule } from '@angular/material/icon';
import { MatSnackBarModule, MatSnackBar } from '@angular/material/snack-bar';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    MatIconModule,
    MatSnackBarModule
  ],
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.css']
})
export class DashboardComponent implements OnInit {
  private api = inject(ApiService);
  private snackBar = inject(MatSnackBar);
  private document = inject(DOCUMENT);

  longUrl = signal('');
  loading = signal(false);
  isFocused = signal(false);
  recentUrls = signal<UrlItem[]>([]);
  latestShortenedUrl = signal<string | null>(null);
  customAlias = signal('');
  selectedTTL = signal(0);

  // Search and Pagination State
  searchText = signal('');
  currentPage = signal(1);
  pageSize = signal(10);

  // Computed filtered list
  filteredUrls = computed(() => {
    const query = this.searchText().toLowerCase().trim();
    const urls = this.recentUrls();
    if (!query) return urls;
    return urls.filter(item => 
      item.short_key.toLowerCase().includes(query) || 
      item.long_url.toLowerCase().includes(query)
    );
  });

  // Computed total pages
  totalPages = computed(() => {
    const total = this.filteredUrls().length;
    const size = this.pageSize();
    return Math.max(1, Math.ceil(total / size));
  });

  // Computed paginated list
  paginatedUrls = computed(() => {
    const urls = this.filteredUrls();
    const size = this.pageSize();
    const page = this.currentPage();
    const startIndex = (page - 1) * size;
    return urls.slice(startIndex, startIndex + size);
  });

  // Helper values for display stats
  entryStart = computed(() => {
    if (this.filteredUrls().length === 0) return 0;
    return (this.currentPage() - 1) * this.pageSize() + 1;
  });

  entryEnd = computed(() => {
    return Math.min(this.currentPage() * this.pageSize(), this.filteredUrls().length);
  });

  ngOnInit() {
    this.loadUrls();
  }

  loadUrls() {
    this.api.getUrls().subscribe({
      next: (urls) => {
        this.recentUrls.set(urls);
        this.currentPage.set(1); // Reset page on new load
      },
      error: (err) => console.error('Failed to load URLs', err)
    });
  }

  onSearchChange(val: string) {
    this.searchText.set(val);
    this.currentPage.set(1); // Reset page to first
  }

  onPageSizeChange(val: string | number) {
    this.pageSize.set(Number(val));
    this.currentPage.set(1);
  }

  goToPage(page: number) {
    if (page >= 1 && page <= this.totalPages()) {
      this.currentPage.set(page);
    }
  }

  shorten() {
    const urlVal = this.longUrl() ? this.longUrl().trim() : '';
    if (!urlVal) {
      this.snackBar.open('Please enter a URL to shorten', 'Close', { duration: 3000 });
      return;
    }

    // 1. URL validation
    if (!urlVal.startsWith('http://') && !urlVal.startsWith('https://')) {
      this.snackBar.open('URL must start with http:// or https://', 'Close', { duration: 3000 });
      return;
    }

    try {
      new URL(urlVal);
    } catch (_) {
      this.snackBar.open('Please enter a valid URL format', 'Close', { duration: 3000 });
      return;
    }

    // 3. Prevent self-referential or loopback URL shortening on client side
    let inputHost = '';
    try {
      inputHost = new URL(urlVal).hostname.toLowerCase();
    } catch (_) {}

    const currentHost = window.location.hostname.toLowerCase();
    if (inputHost === currentHost || inputHost === 'localhost' || inputHost === '127.0.0.1' || inputHost === '::1') {
      this.snackBar.open('Cannot shorten loopback, private, or self-referential URLs', 'Close', { duration: 4000 });
      return;
    }

    // 2. Custom Alias validation
    const aliasVal = this.customAlias() ? this.customAlias().trim() : '';
    if (aliasVal) {
      if (aliasVal.length < 3 || aliasVal.length > 15) {
        this.snackBar.open('Alias must be between 3 and 15 characters long', 'Close', { duration: 3000 });
        return;
      }

      const aliasRegex = /^[a-zA-Z0-9-_]+$/;
      if (!aliasRegex.test(aliasVal)) {
        this.snackBar.open('Alias can only contain alphanumeric characters, dashes, and underscores', 'Close', { duration: 3000 });
        return;
      }

      const blacklist = new Set([
        'api', 'health', 'dashboard', 'analytics', 'architecture', 'swagger', 'expired'
      ]);
      if (blacklist.has(aliasVal.toLowerCase())) {
        this.snackBar.open('Alias is a reserved system route name', 'Close', { duration: 3000 });
        return;
      }
    }

    this.loading.set(true);
    this.api.shortenUrl(urlVal, aliasVal, this.selectedTTL()).subscribe({
      next: (res) => {
        this.longUrl.set('');
        this.customAlias.set('');
        this.selectedTTL.set(0);
        this.latestShortenedUrl.set(res.short_url);
        this.loadUrls();
        this.snackBar.open(`Shortened to: ${res.short_url}`, 'Close', { duration: 5000 });
        this.loading.set(false);
      },
      error: (err) => {
        const errorMsg = err.error?.error || 'Error creating short URL';
        this.snackBar.open(errorMsg, 'Close', { duration: 4000 });
        console.error(err);
        this.loading.set(false);
      }
    });
  }

  copyToClipboard(urlOrKey: string) {
    const url = urlOrKey.startsWith('http://') || urlOrKey.startsWith('https://')
      ? urlOrKey
      : `${this.document.location.origin}/${urlOrKey}`;

    navigator.clipboard.writeText(url).then(() => {
      this.snackBar.open('Copied to clipboard!', 'Close', { duration: 2000 });
    });
  }

  navigateToUrl(urlOrKey: string) {
    const url = urlOrKey.startsWith('http://') || urlOrKey.startsWith('https://')
      ? urlOrKey
      : `${this.document.location.origin}/${urlOrKey}`;
    window.open(url, '_blank');
  }

  deleteUrl(shortKey: string) {
    this.api.deleteUrl(shortKey).subscribe({
      next: () => {
        this.loadUrls();
        const latest = this.latestShortenedUrl();
        if (latest && latest.endsWith('/' + shortKey)) {
          this.latestShortenedUrl.set(null);
        }
        this.snackBar.open('URL deleted successfully', 'Close', { duration: 2000 });
      },
      error: (err) => {
        this.snackBar.open('Failed to delete URL', 'Close', { duration: 3000 });
        console.error(err);
      }
    });
  }
}
