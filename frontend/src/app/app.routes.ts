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
// Force rebuild watcher
