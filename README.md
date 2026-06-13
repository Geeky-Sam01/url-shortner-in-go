# High-Performance URL Shortener

A highly scalable, resume-worthy URL shortener capable of handling 1,000+ reads per second with ultra-low latency redirection.

This project uses a modern distributed architecture, dividing responsibilities between the Edge (Vercel) and a persistent API backend (Railway), connected by a centralized PostgreSQL database (Supabase) and a serverless Redis cache (Upstash).

## Tech Stack

* **Frontend & Edge Redirects**: Angular 20, Angular Material, Vercel Edge Middleware.
* **Backend API & Workers**: Go 1.22, Gin Framework, standard `database/sql` with `lib/pq`.
* **Database**: Supabase (PostgreSQL).
* **Caching & Queues**: Upstash (Serverless Redis over TLS).
* **Deployment**: Vercel (Frontend) and Railway (Backend via Docker).

## Features

1. **Ultra-Low Latency Redirects**: Vercel Edge Middleware intercepts short URLs and checks Upstash Redis directly from the edge. Cache hits return a 302 redirect instantly (sub-10ms) without hitting the backend.
2. **High-Performance Go API**: The backend is written in Go using the Gin framework. It generates unique sequential IDs from PostgreSQL and uses a custom Base62 encoder to guarantee 100% collision-free short URLs.
3. **Asynchronous Analytics**: Click events (referrer, user-agent, IP, country) are pushed to a Redis List queue. A background Go Goroutine batches and inserts them into PostgreSQL to prevent database locks and latency spikes during high traffic.
4. **Sleek Angular Dashboard**: A clean, single-page Angular Material UI to generate new short URLs and view recently created ones.

## Project Structure

* `/backend/` - The Go API server, background worker, database migrations, and caching logic.
* `/frontend/` - The Angular 20 SPA and Vercel `middleware.ts`.

---

## How to Run Locally

### Prerequisites
1. [Go 1.22+](https://golang.org/dl/)
2. [Node.js 20+](https://nodejs.org/en/)
3. A PostgreSQL database URL (e.g., from Supabase)
4. An Upstash Redis REST URL and standard TLS URL.

### 1. Backend Setup

Open a terminal and navigate to the backend directory:
```bash
cd backend
```

Set the required environment variables:
```bash
# Windows PowerShell
$env:DATABASE_URL="postgresql://user:password@host:6543/postgres"
$env:REDIS_URL="rediss://default:password@host:port"
$env:PORT="8080"
$env:FRONTEND_URL="http://localhost:4200"
```

Run the API server (migrations will run automatically on startup):
```bash
go run ./main.go
```
The API will start on `http://localhost:8080`.

### 2. Frontend Setup

Open a new terminal and navigate to the frontend directory:
```bash
cd frontend
```

Install dependencies:
```bash
npm install
```

Configure your environment variables for Edge Middleware (Vercel uses `.env.local` for local dev):
Create a file named `.env.local` inside `/frontend/`:
```env
UPSTASH_REDIS_REST_URL="https://your-upstash-rest-url.upstash.io"
UPSTASH_REDIS_REST_TOKEN="your_upstash_rest_token"
BACKEND_URL="http://localhost:8080/api"
```

Start the Angular development server:
```bash
npm run start
```
The frontend will be available at `http://localhost:4200`.

### Running Tests

To run the Go backend tests (includes mocked Redis and PostgreSQL tests):
```bash
cd backend
go test ./... -v
```
