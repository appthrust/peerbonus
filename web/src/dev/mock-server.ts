// Dev-only API mock: `bun src/dev/mock-server.ts` (or `bun run mock`). Serves
// the peerbonus contract (`/api/peerbonus/v1/*` and `/config`, ./mock-peerbonus,
// with the admin routes ./mock-admin mounts) and the app switcher's
// `GET /launcher` on one port; Vite proxies all three here (vite.config.ts).
import { createMockLauncherHandler, mockLauncherEntries } from '@appthrust/app-launcher'
import './mock-admin'
import { peerbonusRoutes } from './mock-peerbonus'

const port = Number(process.env.PEERBONUS_MOCK_PORT ?? 5241)
const launcher = createMockLauncherHandler(mockLauncherEntries('peerbonus'))

const server = Bun.serve({
  port,
  hostname: '127.0.0.1',
  async fetch(request) {
    const url = new URL(request.url)
    return launcher(request) ?? (await peerbonusRoutes(request, url)) ?? Response.json({ error: 'not_found', message: `no mock for ${request.method} ${url.pathname}` }, { status: 404 })
  },
})

console.log(`peerbonus mock API on http://${server.hostname}:${server.port} (MOCK_ROLE=${process.env.MOCK_ROLE ?? 'Admin'})`)
