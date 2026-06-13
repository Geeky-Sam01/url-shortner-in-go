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
