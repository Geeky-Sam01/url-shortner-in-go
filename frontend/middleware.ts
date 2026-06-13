import { Redis } from '@upstash/redis';

export const config = {
  matcher: [
    '/((?!api|_next/static|_next/image|favicon.ico|assets|styles.css|main.js|polyfills.js).{6,15})',
  ],
};

const redis = new Redis({
  url: process.env['UPSTASH_REDIS_REST_URL'] || '',
  token: process.env['UPSTASH_REDIS_REST_TOKEN'] || '',
});

export default async function middleware(request: Request) {
  const url = new URL(request.url);
  
  // Extract the short key (remove leading slash)
  const key = url.pathname.substring(1);

  // If it doesn't look like our base62 key, skip
  if (!/^[0-9a-zA-Z]{6,15}$/.test(key)) {
    return Response.redirect(url.origin + '/', 302); // Let angular handle or redirect
  }

  try {
    // Check Upstash Redis directly from the edge
    const longUrl = await redis.get<string>(`url:${key}`);
    
    if (longUrl) {
      // Cache HIT: instant redirect
      return Response.redirect(longUrl, 302);
    }
    
    // Cache MISS: Rewrite to the Go backend
    // The backend will query the DB, update the cache, and redirect.
    const backendUrl = process.env['BACKEND_URL'] || 'https://url-shortener-backend.up.railway.app';
    const rewriteUrl = new URL(`${backendUrl}/${key}`);
    return fetch(new Request(rewriteUrl.toString(), request));
    
  } catch (error) {
    console.error('Redis error in edge middleware:', error);
    // On redis error, fallback to Go backend
    const backendUrl = process.env['BACKEND_URL'] || 'https://url-shortener-backend.up.railway.app';
    const rewriteUrl = new URL(`${backendUrl}/${key}`);
    return fetch(new Request(rewriteUrl.toString(), request));
  }
}

