import { TestBed } from '@angular/core/testing';
import { DashboardComponent } from './dashboard.component';
import { ApiService } from '../services/api.service';
import { MatSnackBar } from '@angular/material/snack-bar';
import { of } from 'rxjs';
import { expect, test, describe, beforeEach } from 'vitest';

describe('DashboardComponent QR Code', () => {
  let component: DashboardComponent;
  let apiServiceMock: any;
  let snackBarMock: any;

  beforeEach(() => {
    apiServiceMock = {
      getUrls: () => of([]),
      shortenUrl: () => of({ short_key: 'abc', short_url: 'http://localhost/abc' })
    };
    snackBarMock = {
      open: () => {}
    };

    TestBed.configureTestingModule({
      imports: [DashboardComponent],
      providers: [
        { provide: ApiService, useValue: apiServiceMock },
        { provide: MatSnackBar, useValue: snackBarMock }
      ]
    });

    const fixture = TestBed.createComponent(DashboardComponent);
    component = fixture.componentInstance;
  });

  test('should initialize with null QR signals', () => {
    expect(component.latestShortenedQrDataUrl()).toBeNull();
    expect(component.activePopupUrl()).toBeNull();
    expect(component.activePopupQrDataUrl()).toBeNull();
  });

  test('openQrPopup should set activePopupUrl and activePopupQrDataUrl', async () => {
    component.openQrPopup('testkey');
    expect(component.activePopupUrl()).toContain('testkey');
    await new Promise(resolve => setTimeout(resolve, 100));
    expect(component.activePopupQrDataUrl()).not.toBeNull();
    expect(component.activePopupQrDataUrl()).toContain('data:image/png;base64');
  });

  test('closeQrPopup should clear popup signals', () => {
    component.activePopupUrl.set('http://localhost/testkey');
    component.activePopupQrDataUrl.set('data:image/png;base64,...');
    component.closeQrPopup();
    expect(component.activePopupUrl()).toBeNull();
    expect(component.activePopupQrDataUrl()).toBeNull();
  });
});
