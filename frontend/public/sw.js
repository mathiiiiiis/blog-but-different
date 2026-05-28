const CACHE_NAME = 'liveblog-v1'

const PRECACHE_URLS = [
  '/',
  '/favicon.webp',
]

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(PRECACHE_URLS))
  )
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    )
  )
  self.clients.claim()
})

self.addEventListener('fetch', (event) => {
  // Only handle GET requests for same-origin or cached assets
  if (event.request.method !== 'GET') return

  const url = new URL(event.request.url)

  // Network-first for API/WS calls
  if (url.pathname.startsWith('/api') || url.pathname.startsWith('/ws')) return

  // Cache-first for everything else
  event.respondWith(
    caches.match(event.request).then((cached) => {
      const network = fetch(event.request).then((response) => {
        if (response.ok) {
          caches.open(CACHE_NAME).then((cache) => cache.put(event.request, response.clone()))
        }
        return response
      })
      return cached || network
    })
  )
})
