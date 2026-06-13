# HTTP Graceful Shutdown Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Replace the blocking server startup with an asynchronous `http.Server` in a goroutine and implement graceful shutdown on SIGINT/SIGTERM to drain HTTP requests and stop background workers without data loss.

**Architecture:** Use a signal channel listening to `os.Interrupt` (SIGINT) and `syscall.SIGTERM`. When triggered, a 5-second timeout context is passed to `srv.Shutdown(ctx)` and then `analyticsWorker.Stop()` is called.

**Tech Stack:** Go standard library (`net/http`, `os/signal`, `syscall`, `context`, `time`), Gin Gonic framework.

---

### Task 1: Update Imports and Server Startup in `backend/main.go`

**Files:**
- Modify: `backend/main.go`

**Step 1: Write the implementation plan changes**
- Add `"context"`, `"net/http"`, `"os/signal"`, `"syscall"`, and `"time"` to imports.
- Remove `defer analyticsWorker.Stop()` from initialization to avoid double-stopping (as we will call it explicitly during shutdown).
- Replace `router.Run(":" + cfg.Port)` block with:
  ```go
  srv := &http.Server{
      Addr:    ":" + cfg.Port,
      Handler: router,
  }

  go func() {
      slog.Info("starting server", "port", cfg.Port)
      if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
          slog.Error("server crashed", "error", err)
          os.Exit(1)
      }
  }()

  quit := make(chan os.Signal, 1)
  signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
  <-quit
  slog.Info("shutting down server...")

  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()

  if err := srv.Shutdown(ctx); err != nil {
      slog.Error("server forced to shutdown", "error", err)
  } else {
      slog.Info("server gracefully stopped")
  }

  slog.Info("stopping background analytics worker...")
  analyticsWorker.Stop()

  slog.Info("server exiting")
  ```

**Step 2: Verify compilation**
- Run: `go build` inside the `backend` directory.
- Expected: Successful compilation with no errors.

**Step 3: Run existing tests**
- Run: `go test ./...`
- Expected: All tests pass.
