# High-Performance URL Shortener

A highly scalable, resume-worthy URL shortener capable of handling 1,000+ reads per second with ultra-low latency redirection.

This project uses a modern distributed architecture, dividing responsibilities between the Edge (Vercel) and a persistent API backend (Railway), connected by a centralized PostgreSQL database (Supabase) and a serverless Redis cache (Upstash).

## Tech Stack

* **Frontend & Edge Redirects**: Angular 22, Angular Material, Vercel Edge Middleware.
* **Backend API & Workers**: Go 1.22, Gin Framework, standard `database/sql` with `lib/pq`.
* **Database**: Supabase (PostgreSQL).
* **Caching & Queues**: Upstash (Serverless Redis over TLS).
* **Deployment**: Vercel (Frontend) and Railway (Backend via Docker).

## Features

1. **Ultra-Low Latency Redirects**: Vercel Edge Middleware intercepts short URLs and checks Upstash Redis directly from the edge. Cache hits return a 302 redirect instantly (sub-10ms) without hitting the backend.
2. **High-Performance Go API**: The backend is written in Go using the Gin framework. It generates unique sequential IDs from PostgreSQL and uses a custom Base62 encoder to guarantee 100% collision-free short URLs.
3. **Asynchronous Analytics**: Click events (referrer, user-agent, IP, country) are pushed to a Redis List queue. A background Go Goroutine batches and inserts them into PostgreSQL to prevent database locks and latency spikes during high traffic.
4. **Resilient Failover & Requeueing**: If PostgreSQL transactions fail during batch insertions, the background analytics worker automatically requeues click events back to Upstash Redis (`RPush` pipeline) to prevent analytics data loss.
5. **Graceful Shutdown**: The Go backend implements graceful shutdown on `SIGINT` or `SIGTERM`, draining pending HTTP requests (with a 5-second timeout) and safely stopping background workers.
6. **Sleek Angular Dashboard**: A clean, single-page Angular Material UI to generate new short URLs and view recently created ones.

## Project Structure

* `/backend/` - The Go API server, background worker, database migrations, and caching logic.
* `/frontend/` - The Angular 22 SPA and Vercel `middleware.ts`.

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

---

## Deployment

### 1. Deploying the Backend (Railway)
1. In the Railway dashboard, select **New Project** -> **Deploy from GitHub**.
2. Under the service settings, set the **Root Directory** to `/backend`.
3. Set the following environment variables:
   - `DATABASE_URL`: Your Supabase connection string.
   - `REDIS_URL`: Your Upstash Redis connection string.
   - `ENV`: `production`
   - `PORT`: `8080`
   - `FRONTEND_URL`: The URL of your Vercel deployment (update this after deploying the frontend).

### 2. Deploying the Frontend (Vercel)
1. Import your repository into Vercel.
2. In the import settings:
   - Set **Root Directory** to `frontend`.
   - Ensure **Framework Preset** is set to `Angular`.
3. Configure the following environment variables:
   - `UPSTASH_REDIS_REST_URL`: Your Upstash REST URL.
   - `UPSTASH_REDIS_REST_TOKEN`: Your Upstash REST Token.
   - `BACKEND_URL`: Your Railway deployment URL.
4. Click **Deploy**. Copy the deployment URL and update the `FRONTEND_URL` on Railway.
