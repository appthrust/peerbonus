# Peer Bonus

Peer Bonus is AppThrust's points-and-thanks application. Colleagues send weekly
points and recognition, and administrators close each month into payroll-ready
statements. The Go server serves the React SPA, `/api/peerbonus/v1/*`, `/config`,
`/launcher`, `/healthz`, `/readyz`, and `/releasez` from one origin. PostgreSQL
stores application data; Directory remains a separate app called over HTTP.
The SPA uses the session cookie behind the OIDC edge and never handles tokens.
Optional Slack credentials enable announcements, direct messages, the
`/peerbonus` command, and giving inside a conversation: mentioning the bot
(`@Peer Bonus @recipient 30 message`; the bot asks for the amount when it is
omitted) or adding an admin-chosen reaction. Slack reaches the server through
the `slack` inbound endpoint declared in `catalog/entry.yaml` (`/slack/*`).

A sender can give only once from the same Slack message, regardless of the emoji
or whether the give started with a mention. A mention can still give to up to
five recipients atomically; retries replay that entire original batch. Stored
gives are unique by (channel, message timestamp, sender, recipient), with emoji
retained only for reference. Removing a reaction does not refund points or allow
another give. This replaces the per-emoji RL2 rule in `reaction-give.md`.

## Develop

Use Go 1.26 or newer and Bun 1.4.1. The server depends on the public
[`github.com/appthrust/appserver-go`](https://github.com/appthrust/appserver-go)
`v0.2.0` module, without a replacement or platform checkout. The shared launcher
and translations use published GitHub Packages, not workspace packages.

```sh
go build ./... && go test ./...
go vet ./...
cd web
export NODE_AUTH_TOKEN="$(gh auth token)"
bun install --frozen-lockfile
bun run typecheck
bun run build:image
```

The npm token needs package read access. `web/.npmrc` reads `NODE_AUTH_TOKEN`;
do not commit a token. `web/bun.lock` is committed and used unchanged by CI and
Docker builds. The SPA output is served at `/srv/peerbonus` in the image.

From `web/`, run these in separate terminals:

```sh
bun run mock   # in-memory API on 127.0.0.1:5241 (PEERBONUS_MOCK_PORT)
bun run dev    # Vite on :5240 (PEERBONUS_PORT); proxies /api, /config, /launcher
```

For a real server, set `PEERBONUS_API_UPSTREAM=<origin>` and either
`PEERBONUS_DEV_BEARER=<token>` for a local Go server or
`PEERBONUS_DEV_COOKIE_FILE=<file>` containing the Cookie header of a signed-in
browser session. The proxy rereads the cookie file per request. Run the server
from the repository root with `go run ./cmd/server --static-dir web/dist`, with
`PLATFORM_API_URL`, `PLATFORM_PROJECT_ID`, and the `PLATFORM_*` OIDC configuration
set for the target project. Configure `PEERBONUS_DATABASE_URL` or `DATABASE_URL`.
The default listener is `:8080`; `PEERBONUS_STATIC_DIR` overrides the default
static directory, and `--static-dir` overrides the environment.

Directory access is resolved through the platform app link or
`PEERBONUS_DIRECTORY_URL`; `PEERBONUS_DIRECTORY_PUBLIC_URL` supplies browser
links. `PEERBONUS_PUBLIC_URL` must be an HTTPS origin. Slack uses optional
`PEERBONUS_SLACK_BOT_TOKEN` and `PEERBONUS_SLACK_SIGNING_SECRET` credentials.

The UI mock signs in as 山田 (admin) unless `MOCK_ROLE` differs from `Admin`.
The recipient fixtures include two small teams, `sales/team-a` and
`projects/ceatec`. Mock reset, caller switching, Directory availability, and
Slack configuration controls are documented in [web/README.md](web/README.md).

PostgreSQL integration tests skip unless `BUILTIN_APPSERVER_TEST_DATABASE_URL`
or `BUILDER_CONSOLE_TEST_DATABASE_URL` supplies a test database that permits
isolated schema creation. The copied non-database tests and publisher tests
need no live services:

```sh
bun test hack/
cd web
bun run test
```

Build the image from the repository root:

```sh
NODE_AUTH_TOKEN="$(gh auth token)" docker build --platform linux/amd64 \
  --secret id=NODE_AUTH_TOKEN,env=NODE_AUTH_TOKEN -t peerbonus-local .
```

The image builds the architecture-independent SPA natively, cross-compiles Go,
and runs `/peerbonus-server` as user `65532` in distroless with port `8080` exposed.
No JavaScript runtime or build-time token is included in the final image.

## Release

The existing served catalog version is `0.1.0` on stream `0.1`. The first
independent release is **`0.2.0`**, on stream **`0.2`**. The committed catalog
contains only the live compatibility floor, never a placeholder for a new image.
After reviewing the repository and package prerequisites, release with a SemVer
tag such as `v0.2.0`.

`.github/workflows/publish.yml` runs on `ubuntu-24.04` and makes a single
multi-platform build for Linux amd64 and arm64. It pushes
`ghcr.io/appthrust/peerbonus:<version>`, resolves the image index digest, and
signs it with Cosign keyless signing. Ensure the resulting GHCR package is public.
The repository's `GITHUB_TOKEN` needs packages write access; OIDC needs
`id-token: write`. Both `@appthrust/app-launcher` and
`@appthrust/platform-dashboard-i18n` must grant this repository package read
access. Docker receives npm authentication only as a BuildKit secret.

Repository configuration:

- Secret `CLOUD_CATALOG_ADMIN_TOKEN`.
- Variable `CLOUD_API_URL` (defaults to `https://api.appthrust.com`).
- Organization secrets `APPTHRUST_GITHUB_ACTIONS_APP_ID` and
  `APPTHRUST_GITHUB_ACTIONS_APP_PRIVATE_KEY`, with the App installed on
  `appthrust/cloud` and allowed to send repository dispatches.

The publisher reads the existing Cloud catalog entry, merges its version history
with `catalog/entry.yaml`, and PUTs `/api/v1/catalog/entries/peerbonus`. It then
POSTs `/api/v1/catalog/publish`, accepting HTTP 202 `{id, state: "queued"}` or
HTTP 200 `{id, state: "snapshot"}`. Only `snapshot` requires the workflow to
mint a Cloud installation token and dispatch `catalog-publish`. The concurrency
group `peerbonus-catalog-publish` serializes this read/merge/write sequence.

Publication is not complete when dispatch succeeds. `await` checks the catalog's
`index.json` layer and digest through ORAS, requires the exact Peer Bonus version
and image digest, and verifies the catalog signature against GitHub's OIDC issuer
and Cloud's `catalog-publish.yml@refs/heads/main` identity. It checks every 15
seconds for at most 10 minutes; failure or timeout fails the tag workflow.

```sh
bun hack/publish-entry.ts submit --version 0.2.0 --digest sha256:<image-index-digest>
bun hack/publish-entry.ts await --version 0.2.0 --digest sha256:<image-index-digest>
```

`submit` requires the Cloud token; `await` requires ORAS 1.3.0 and Cosign 3.1.3,
not a Cloud token. For manual submission, a `snapshot` response also needs the
Cloud repository dispatch before awaiting publication.

## Catalog compatibility

`catalog/entry.yaml` was read from the `peerbonus` ApplicationStackDefinition on
`appthrust-ink-pmc001`. It preserves the served `0.1.0` image digest, both web and
PostgreSQL components, the database connection, optional Slack credential slots,
and the automatic Directory dependency.

A release appends `ghcr.io/appthrust/peerbonus@sha256:<index-digest>` with
`served: true`, `default: true`, and its major/minor stream. Earlier versions
remain served with their default cleared. Image identities are immutable:
repeating a version/digest is idempotent; replacing an existing digest fails.
Existing installations on `ghcr.io/appthrust/platform/peerbonus` need one explicit
version update with `replaceRepository: true`; later `follow` updates stay within
the selected stream. `/releasez` reports `PEERBONUS_RELEASE_VERSION` and
`PEERBONUS_ARTIFACT_DIGEST` without a platform release closure.

## Provenance

Imported from `appthrust/platform` commit
`27347914ee15817f27a42055160b42506edc73e6`: `cmd/peerbonus-server/` is now
`cmd/server/`, and `dashboard/packages/peerbonus/` is now `web/`. Server behavior,
Directory HTTP calls, migrations, and tests are retained. Release tooling follows
[`appthrust/directory`](https://github.com/appthrust/directory).

- [Peer Bonus design and API contract](https://github.com/appthrust/platform/blob/main/design/platform/builtin-app-peerbonus/README.md)
- [First-party app repository migration](https://github.com/appthrust/platform/blob/main/design/platform/marketplace-apps/first-party-app-repositories.md)
