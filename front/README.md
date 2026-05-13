# urlo Frontend

React 19 + TypeScript + Vite frontend for `urlo`.

## Implemented Pages

- `/` landing page for shortening URLs
- `/dashboard` link management dashboard
- `/analytics` link picker for analytics
- `/analytics/:code` analytics details for a link
- `/settings` local frontend settings

## Implemented Features

- Create short links from the browser
- Optional custom aliases with live availability check
- Auto-generated code length toggle for short or long codes
- Google sign-in integration when the backend enables it
- Local storage fallback for anonymous users
- Link listing with search/filter
- Refresh stats for one link or all links
- Edit destination URL
- Delete links
- Enable or disable links with an optional reason
- View analytics and recent click information
- Generate and download QR codes
- Client-side API base URL override in settings

## Scripts

```bash
pnpm install
pnpm dev
pnpm build
pnpm lint
pnpm preview
```

## Environment

- `VITE_API_BASE_URL`: optional backend base URL prefix
- `VITE_GOOGLE_CLIENT_ID`: enables the Google sign-in button in the UI

If `VITE_API_BASE_URL` is unset, the app uses the current origin. A user can also override the API base URL in the settings page, which is stored in local storage.
