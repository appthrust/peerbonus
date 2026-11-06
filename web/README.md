# @appthrust/peerbonus

The Peer Bonus built-in app SPA (`peerbonus`). `cmd/server` serves the built
bundle together with `/api/peerbonus/v1/*`, `/config` and `/launcher` from one origin
behind the OIDC edge; the SPA only uses the session cookie and never handles tokens.
Design and API contract: [platform design](https://github.com/appthrust/platform/blob/main/design/platform/builtin-app-peerbonus/README.md).

## Run

```sh
bun run mock   # in-memory API on 127.0.0.1:5241 (PEERBONUS_MOCK_PORT)
bun run dev    # Vite on :5240 (PEERBONUS_PORT), proxies /api, /config, /launcher
```

Against a real server instead of the mock: `PEERBONUS_API_UPSTREAM=<origin>` plus either
`PEERBONUS_DEV_BEARER=<token>` (a local Go server) or `PEERBONUS_DEV_COOKIE_FILE=<file>`
holding the `Cookie` header of a signed-in browser session.

The mock (`src/dev/mock-peerbonus.ts`, admin routes in `src/dev/mock-admin.ts`) signs
you in as 山田 (admin) unless `MOCK_ROLE` is not `Admin`. 山田 is directly in two small
teams (`sales/team-a`, `projects/ceatec`), so the recipient picker stacks them; tabs
appear only past 3 teams or 12 people in all, which no fixture reaches (see the picker
tests). Controls:

| Request | Effect |
|---|---|
| `POST /api/peerbonus/v1/__mock/reset` | reseed (posts over the current and two previous months) |
| `POST /api/peerbonus/v1/__mock/caller` `{memberId, admin?}` | switch the signed-in member |
| `POST /api/peerbonus/v1/__mock/directory` `{up}` | take Directory down (503 `directory_unavailable`) |
| `POST /api/peerbonus/v1/__mock/slack` `{botToken?, signingSecret?, team?, inbound?, invite?}` | Slack environment for `/admin/slack`: credentials present, the bot token's workspace (`{teamId, teamName}`; another one turns a connected integration into `team_changed`), the Phase B inbound endpoint (`{url, ready, reason}` or null), and `/invite` the bot to a channel id. Slack starts `configured` (token set, not verified); reset starts it over |

## Check

```sh
bun run typecheck && bun run test && bun run build
```
