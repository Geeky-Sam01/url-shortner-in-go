| Status | Task | Description |
| :---: | :--- | :--- |
| `[x]` | Task 1: Global Styles (`styles.css`) | Update dynamic backgrounds, refine glassmorphism, add micro-animations, and create Bento Grid utilities. |
| `[x]` | Task 2: Main Component Markup (`app.component.html`) | Refactor to Bento Grid Layout, add interactive glass hero fold, dynamic input console, scrollable left card, and right card analytics placeholder. |
| `[x]` | Task 1 (Backend): Database Schema & Migration Update | Add a database index on the `clicks.url_id` foreign key. |
| `[x]` | Task 2 (Backend): Redis Helper & Cost Optimization | Check queue length before popping in `PopAnalyticsEvents` to avoid empty RPop commands. |
| `[x]` | Task 3 (Backend): Background Worker Data Loss Recovery | Add transaction fallback recovery to push events back to Redis on database batch insert failure. |
| `[x]` | Task 4 (Backend): HTTP Graceful Shutdown Implementation | Set up a SIGINT/SIGTERM signal channel and trigger graceful shutdown of Gin server and background workers. |
| `[x]` | Task 5 (Backend): Infinite Redirect Loops & Atomic Rate Limiter | Prevent self-referential URL shortening and replace unsafe Incr+Expire rate limit logic with atomic Redis Lua script. |

