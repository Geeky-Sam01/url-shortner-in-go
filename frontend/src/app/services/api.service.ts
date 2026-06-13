import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../environments/environment';

export interface ShortenResponse {
  short_key: string;
  short_url: string;
}

export interface UrlItem {
  id: number;
  short_key: string;
  long_url: string;
  created_at: string;
}

@Injectable({
  providedIn: 'root'
})
export class ApiService {
  private http = inject(HttpClient);
  private apiUrl = environment.apiUrl;

  shortenUrl(longUrl: string): Observable<ShortenResponse> {
    return this.http.post<ShortenResponse>(`${this.apiUrl}/shorten`, { url: longUrl });
  }

  getUrls(): Observable<UrlItem[]> {
    return this.http.get<UrlItem[]>(`${this.apiUrl}/urls`);
  }

  deleteUrl(shortKey: string): Observable<any> {
    return this.http.delete(`${this.apiUrl}/urls/${shortKey}`);
  }
}
