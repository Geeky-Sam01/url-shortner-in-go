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
