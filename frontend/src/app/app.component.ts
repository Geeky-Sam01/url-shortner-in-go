import { Component, inject, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ApiService, UrlItem } from './services/api.service';

import { MatToolbarModule } from '@angular/material/toolbar';
import { MatCardModule } from '@angular/material/card';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatTableModule } from '@angular/material/table';
import { MatIconModule } from '@angular/material/icon';
import { MatSnackBarModule, MatSnackBar } from '@angular/material/snack-bar';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    MatToolbarModule,
    MatCardModule,
    MatFormFieldModule,
    MatInputModule,
    MatButtonModule,
    MatTableModule,
    MatIconModule,
    MatSnackBarModule
  ],
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.css']
})
export class AppComponent implements OnInit {
  private api = inject(ApiService);
  private snackBar = inject(MatSnackBar);

  longUrl = '';
  loading = false;
  isFocused = false;
  recentUrls: UrlItem[] = [];
  displayedColumns: string[] = ['short_key', 'long_url', 'created_at', 'actions'];

  ngOnInit() {
    this.loadUrls();
  }

  loadUrls() {
    this.api.getUrls().subscribe({
      next: (urls) => this.recentUrls = urls,
      error: (err) => console.error('Failed to load URLs', err)
    });
  }

  shorten() {
    if (!this.longUrl) return;
    this.loading = true;
    this.api.shortenUrl(this.longUrl).subscribe({
      next: (res) => {
        this.longUrl = '';
        this.loadUrls();
        this.snackBar.open(`Shortened to: ${res.short_url}`, 'Close', { duration: 5000 });
      },
      error: (err) => {
        this.snackBar.open('Error creating short URL', 'Close', { duration: 3000 });
        console.error(err);
      },
      complete: () => this.loading = false
    });
  }

  copyToClipboard(shortKey: string) {
    const url = `${window.location.origin}/${shortKey}`;
    navigator.clipboard.writeText(url).then(() => {
      this.snackBar.open('Copied to clipboard!', 'Close', { duration: 2000 });
    });
  }

  deleteUrl(shortKey: string) {
    this.api.deleteUrl(shortKey).subscribe({
      next: () => {
        this.loadUrls();
        this.snackBar.open('URL deleted successfully', 'Close', { duration: 2000 });
      },
      error: (err) => {
        this.snackBar.open('Failed to delete URL', 'Close', { duration: 3000 });
        console.error(err);
      }
    });
  }
}
