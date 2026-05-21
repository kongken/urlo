# urlo

[![Go](https://github.com/kongken/urlo/actions/workflows/go.yml/badge.svg)](https://github.com/kongken/urlo/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/kongken/urlo/branch/main/graph/badge.svg)](https://codecov.io/gh/kongken/urlo)

**urlo** is a lightweight URL shortener for creating, managing, and tracking short links.

It includes a web dashboard for everyday use and an API for automation/integration.

## What you can do

- Create short links from long URLs
- Choose an optional custom short code
- Set links to expire after a period of time, or keep them forever
- Copy and share generated short links and QR codes
- Manage your links from a dashboard
- Enable, disable, edit, or delete links
- View click analytics such as trends, referrers, browsers, devices, and locations
- Use local browser storage anonymously, or sign in when auth is configured

## Quick start

Run the backend service:

```bash
make run
```

By default, urlo uses the local [config.yaml](config.yaml). The default setup is suitable for local development: in-memory storage, no login requirement, and no external click-log service.

Then start the web app:

```bash
cd front
pnpm install
pnpm dev
```

Open the Vite dev URL shown in the terminal. If the frontend and backend are on different origins, set the API base URL in either of these ways:

- Environment variable before building/running the frontend: `VITE_API_BASE_URL=http://localhost:8080`
- In the app: **Settings → API Base URL**

## Basic usage

### Create a short link

1. Open the web app.
2. Paste a long URL.
3. Optionally enter a custom code.
4. Click **Shorten URL**.
5. Copy the generated link or download/share the QR code.

### Manage links

Go to **Dashboard** to:

- Search your links
- Refresh click counts
- Edit the destination URL
- Change expiration settings
- Enable or disable a link
- Delete one or multiple links

### View analytics

Open **Analytics** from a link to inspect:

- Total and recent clicks
- Daily trend
- Top referrers
- Browsers and devices
- Location breakdown when available

You can filter analytics by all time, last 24 hours, last 7 days, or last 30 days.

### Local data

When using urlo without signing in, created links are saved in the browser. In **Settings**, you can:

- Export local links as JSON
- Import a previous export
- Clear locally stored links
- Test the configured API connection

## Configuration

Most deployments only need to adjust [config.yaml](config.yaml):

- `base_url` — public URL used when generating short links
- `storage` — in-memory storage for local/dev, or S3-compatible storage for persistence
- `auth` — optional Google sign-in
- `clicks` — optional click-event logging
- `rate_limit` — optional public API rate limiting

See comments and examples in [config.yaml](config.yaml) for the available settings.

## API

For integrations and automation, see the API documentation:

- [API overview](docs/api/README.md)
- [Auth](docs/api/auth.md)
- [URLs](docs/api/urls.md)
- [Clicks and analytics](docs/api/clicks.md)
- [Redirect behavior](docs/api/redirect.md)
- [gRPC](docs/api/grpc.md)

## Development

Common checks:

```bash
go test ./...
cd front && pnpm lint && pnpm build
```

Other useful commands:

```bash
make build   # build backend binary
make proto   # regenerate protobuf code
```

The frontend lives in [front/](front/). Its [front/go.mod](front/go.mod) file is intentional: it keeps root-level Go commands from scanning frontend dependencies such as `node_modules`.
