import { useMemo, useState } from "react"
import { Check, Copy, Hash, Search, X } from "lucide-react"
import { cn } from "@/lib/utils"
import { getApiBaseUrl } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"

type Method = "GET" | "POST" | "PATCH" | "DELETE"

const methodStyles: Record<Method, string> = {
  GET: "bg-emerald-500/15 text-emerald-700 ring-emerald-500/30 dark:text-emerald-400",
  POST: "bg-sky-500/15 text-sky-700 ring-sky-500/30 dark:text-sky-400",
  PATCH: "bg-amber-500/15 text-amber-700 ring-amber-500/30 dark:text-amber-400",
  DELETE: "bg-red-500/15 text-red-700 ring-red-500/30 dark:text-red-400",
}

const allMethods: Method[] = ["GET", "POST", "PATCH", "DELETE"]

interface Param {
  name: string
  type: string
  required: boolean
  notes: string
}

interface Endpoint {
  method: Method
  path: string
  title: string
  auth?: string
  description: string
  params?: Param[]
  paramsLabel?: string
  curl: string
  response?: { title?: string; body: string }
  errors?: string
}

interface Group {
  id: string
  title: string
  description?: string
  endpoints: Endpoint[]
}

function CodeBlock({ code }: { code: string }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      /* clipboard unavailable */
    }
  }
  return (
    <div className="group relative overflow-hidden rounded-lg border border-border bg-muted/40 text-xs">
      <button
        type="button"
        onClick={copy}
        className="absolute right-2 top-2 rounded-md border border-border bg-background p-1.5 text-muted-foreground shadow-sm transition-colors hover:text-foreground"
        aria-label="Copy to clipboard"
        title="Copy to clipboard"
      >
        {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
      </button>
      <pre className="overflow-x-auto p-4 font-mono leading-relaxed text-foreground/90">
        <code>{code}</code>
      </pre>
    </div>
  )
}

function MethodBadge({ method }: { method: Method }) {
  return (
    <span
      className={cn(
        "inline-flex h-5 w-fit shrink-0 items-center justify-center rounded-md px-2 font-mono text-xs font-semibold ring-1 ring-inset",
        methodStyles[method],
      )}
    >
      {method}
    </span>
  )
}

function ParamsTable({ title, params }: { title: string; params: Param[] }) {
  return (
    <div>
      <p className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </p>
      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full text-xs min-w-[520px]">
          <thead>
            <tr className="border-b border-border bg-muted/40 text-left">
              <th className="px-3 py-2 font-medium">Name</th>
              <th className="px-3 py-2 font-medium">Type</th>
              <th className="px-3 py-2 font-medium">Required</th>
              <th className="px-3 py-2 font-medium">Notes</th>
            </tr>
          </thead>
          <tbody>
            {params.map((p) => (
              <tr key={p.name} className="border-b border-border last:border-0">
                <td className="px-3 py-2 font-mono text-foreground">{p.name}</td>
                <td className="px-3 py-2 font-mono text-muted-foreground">{p.type}</td>
                <td className="px-3 py-2">
                  {p.required ? (
                    <span className="text-xs font-medium text-destructive">yes</span>
                  ) : (
                    <span className="text-xs text-muted-foreground">no</span>
                  )}
                </td>
                <td className="px-3 py-2 text-muted-foreground">{p.notes}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function EndpointCard({ ep }: { ep: Endpoint }) {
  return (
    <Card id={ep.path} className="scroll-mt-24">
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <MethodBadge method={ep.method} />
          <code className="font-mono text-sm font-medium text-foreground">{ep.path}</code>
          {ep.auth && (
            <Badge variant="secondary" className="font-mono">
              {ep.auth}
            </Badge>
          )}
        </div>
        <CardTitle className="text-sm">{ep.title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{ep.description}</p>
        {ep.params && (
          <ParamsTable title={ep.paramsLabel ?? "Parameters"} params={ep.params} />
        )}
        <div>
          <p className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Example
          </p>
          <CodeBlock code={ep.curl} />
        </div>
        {ep.response && (
          <div>
            <p className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {ep.response.title ?? "Response"}
            </p>
            <CodeBlock code={ep.response.body} />
          </div>
        )}
        {ep.errors && (
          <p className="text-xs text-muted-foreground">
            <span className="font-medium text-foreground">Errors: </span>
            {ep.errors}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

const groups: Group[] = [
  {
    id: "auth",
    title: "Authentication",
    description:
      "Google ID-token login. The API issues an HMAC-signed JWT in an HTTP-only session cookie (default name `urlo_session`). Requests attach it automatically when credentials include.",
    endpoints: [
      {
        method: "POST",
        path: "/api/v1/auth/google",
        title: "Exchange Google ID token",
        description:
          "Verify a Google ID token and start a session. Sets an HTTP-only `urlo_session` cookie on success.",
        params: [
          {
            name: "id_token",
            type: "string",
            required: true,
            notes: "ID token obtained from Google Sign-In",
          },
        ],
        curl: `curl -X POST ${"BASE"}/api/v1/auth/google \\
  -H 'Content-Type: application/json' \\
  -d '{"id_token":"<google_id_token>"}'`,
        response: {
          body: `{ "user": { "sub": "1234…", "email": "you@example.com", "name": "You" } }`,
        },
        errors: "400 (missing id_token), 401 (invalid token), 503 (auth disabled)",
      },
      {
        method: "POST",
        path: "/api/v1/auth/logout",
        title: "Clear session",
        description: "Clears the session cookie. Returns 204 No Content.",
        curl: `curl -X POST ${"BASE"}/api/v1/auth/logout`,
        errors: "503 when auth is disabled on the server",
      },
      {
        method: "GET",
        path: "/api/v1/auth/me",
        title: "Current user",
        description: "Returns the user attached to the current session.",
        curl: `curl ${"BASE"}/api/v1/auth/me`,
        response: {
          body: `{ "user": { "sub": "1234…", "email": "you@example.com", "name": "You" } }`,
        },
        errors: "401 (no/invalid session), 503 (auth disabled)",
      },
    ],
  },
  {
    id: "urls",
    title: "URLs",
    description:
      "Create, resolve, and manage short links. See Authentication for ownership rules.",
    endpoints: [
      {
        method: "POST",
        path: "/api/v1/urls",
        title: "Shorten",
        auth: "optional",
        description:
          "Create a new short link. The server generates a 6-char code unless `custom_code` is provided. When the caller is authenticated, the new link is tagged with their `sub` as owner.",
        paramsLabel: "Request body",
        params: [
          {
            name: "long_url",
            type: "string",
            required: true,
            notes: "Must parse as a valid URL",
          },
          {
            name: "custom_code",
            type: "string",
            required: false,
            notes: "[A-Za-z0-9]{1,32}; 409 if it already exists",
          },
          {
            name: "ttl_seconds",
            type: "integer",
            required: false,
            notes: ">= 0. 0 (default) means no expiration",
          },
          {
            name: "code_length",
            type: "integer",
            required: false,
            notes:
              "Length of auto-generated code in [6, 32]. 0 (default) uses the server-configured length. Ignored when custom_code is set.",
          },
        ],
        curl: `curl -X POST ${"BASE"}/api/v1/urls \\
  -H 'Content-Type: application/json' \\
  -d '{"long_url":"https://example.com/foo","ttl_seconds":3600}'`,
        response: {
          title: "201 Created — ShortLink",
          body: `{
  "code": "aB3xQ7",
  "long_url": "https://example.com/foo",
  "short_url": "http://localhost:8080/aB3xQ7",
  "created_at": "2026-04-30T01:00:00Z",
  "expires_at": "2026-04-30T02:00:00Z",
  "visit_count": 0
}`,
        },
        errors: "400 (invalid body / long_url / custom_code / ttl_seconds / code_length), 409 (custom_code used), 429 (rate-limited)",
      },
      {
        method: "GET",
        path: "/api/v1/urls",
        title: "List my links",
        auth: "required",
        description: "Lists short links owned by the authenticated caller. Expired links are omitted.",
        curl: `curl --cookie 'urlo_session=…' ${"BASE"}/api/v1/urls`,
        response: {
          body: `{ "links": [ { "code": "aB3xQ7", "long_url": "…", … } ] }`,
        },
        errors: "401 (login required)",
      },
      {
        method: "GET",
        path: "/api/v1/urls/:code",
        title: "Resolve",
        description:
          "Look up a short link by code. **Increments `visit_count` by 1** on success. Use this when you need the JSON details.",
        curl: `curl ${"BASE"}/api/v1/urls/aB3xQ7`,
        response: {
          title: "200 OK — ShortLink",
          body: `{ "code": "aB3xQ7", "long_url": "https://example.com/foo", "visit_count": 12 }`,
        },
        errors: "404 (not found / expired / disabled)",
      },
      {
        method: "GET",
        path: "/api/v1/urls/:code/lookup",
        title: "Lookup",
        description:
          "Alias of Resolve for clients that prefer an explicit lookup route. Same response and side effects (includes visit_count increment).",
        curl: `curl ${"BASE"}/api/v1/urls/aB3xQ7/lookup`,
        response: {
          body: `{ "code": "aB3xQ7", "long_url": "https://example.com/foo", "visit_count": 12 }`,
        },
        errors: "404 (not found / expired / disabled)",
      },
      {
        method: "GET",
        path: "/api/v1/urls/:code/stats",
        title: "GetStats",
        auth: "owner",
        description:
          "Same payload as Resolve, but does **not** increment `visit_count`. If the link has an owner, the caller must be authenticated as that owner.",
        curl: `curl ${"BASE"}/api/v1/urls/aB3xQ7/stats`,
        response: {
          title: "200 OK — ShortLink",
          body: `{ "code": "aB3xQ7", "long_url": "https://example.com/foo", "visit_count": 12 }`,
        },
        errors: "403 (not owner), 404 (not found)",
      },
      {
        method: "PATCH",
        path: "/api/v1/urls/:code",
        title: "Update link",
        auth: "owner",
        description:
          "Update mutable fields of a short link. At least one field must be provided.",
        paramsLabel: "Request body",
        params: [
          {
            name: "long_url",
            type: "string",
            required: false,
            notes: "New destination URL",
          },
          {
            name: "ttl_seconds",
            type: "integer",
            required: false,
            notes: "0 clears expiration, >0 resets expiration from now",
          },
        ],
        curl: `curl -X PATCH --cookie 'urlo_session=…' ${"BASE"}/api/v1/urls/aB3xQ7 \\
  -H 'Content-Type: application/json' \\
  -d '{"long_url":"https://example.com/new-target"}'`,
        response: {
          title: "200 OK — updated ShortLink",
          body: `{ "code": "aB3xQ7", "long_url": "https://example.com/new-target", "visit_count": 12 }`,
        },
        errors: "400 (invalid body / invalid URL / no fields), 403 (not owner), 404 (not found)",
      },
      {
        method: "PATCH",
        path: "/api/v1/urls/:code/status",
        title: "Enable / disable link",
        auth: "owner",
        description:
          "Toggle link availability without deleting it. Disabled links resolve as 404.",
        paramsLabel: "Request body",
        params: [
          {
            name: "disabled",
            type: "boolean",
            required: true,
            notes: "true disables, false enables",
          },
          {
            name: "reason",
            type: "string",
            required: false,
            notes: "Optional reason (stored for admin/debug context)",
          },
        ],
        curl: `curl -X PATCH --cookie 'urlo_session=…' ${"BASE"}/api/v1/urls/aB3xQ7/status \\
  -H 'Content-Type: application/json' \\
  -d '{"disabled":true,"reason":"abuse"}'`,
        response: {
          body: `{ "code": "aB3xQ7", "disabled": true, "reason": "abuse" }`,
        },
        errors: "400 (invalid body), 403 (not owner), 404 (not found)",
      },
      {
        method: "GET",
        path: "/api/v1/urls/:code/status",
        title: "Read enable/disable status",
        auth: "owner",
        description: "Return the persisted status for a short link.",
        curl: `curl --cookie 'urlo_session=…' ${"BASE"}/api/v1/urls/aB3xQ7/status`,
        response: {
          body: `{ "code": "aB3xQ7", "disabled": true, "reason": "abuse" }`,
        },
        errors: "403 (not owner), 404 (not found)",
      },
      {
        method: "GET",
        path: "/api/v1/urls/:code/availability",
        title: "Check custom code availability",
        description: "Check whether a custom code is currently free to use.",
        params: [
          {
            name: "code",
            type: "string",
            required: true,
            notes: "Must match code rules ([A-Za-z0-9]{1,32})",
          },
        ],
        curl: `curl '${"BASE"}/api/v1/urls/availability?code=launch'`,
        response: {
          body: `{ "code": "launch", "available": true }`,
        },
        errors: "400 (missing/invalid code)",
      },
      {
        method: "DELETE",
        path: "/api/v1/urls/:code",
        title: "Delete",
        auth: "owner",
        description:
          "Remove a short link. If the link has an owner, the caller must be authenticated as that owner.",
        curl: `curl -X DELETE --cookie 'urlo_session=…' ${"BASE"}/api/v1/urls/aB3xQ7`,
        response: {
          title: "204 No Content",
          body: "",
        },
        errors: "403 (not owner), 404 (not found)",
      },
    ],
  },
  {
    id: "clicks",
    title: "Clicks & analytics",
    description: "Click event log and aggregate analytics for a short link.",
    endpoints: [
      {
        method: "GET",
        path: "/api/v1/urls/:code/clicks",
        title: "ListClicks",
        auth: "owner",
        description:
          "Return recent click events for a short link, newest first. Returns [] when click logging is disabled.",
        params: [
          { name: "page_size", type: "integer", required: false, notes: "Default 50, capped at 500" },
          {
            name: "page_token",
            type: "string",
            required: false,
            notes: "Opaque cursor from a previous response",
          },
        ],
        curl: `curl --cookie 'urlo_session=…' '${"BASE"}/api/v1/urls/aB3xQ7/clicks?page_size=20'`,
        response: {
          title: "200 OK",
          body: `{
  "events": [
    {
      "id": "1714521600123-0",
      "code": "aB3xQ7",
      "ts": "2026-05-01T16:00:00Z",
      "ip_hash": "9c1185a5c5e9fc54",
      "referrer": "https://www.google.com/",
      "referrer_host": "www.google.com",
      "user_agent": "Mozilla/5.0 …",
      "browser": "Chrome",
      "os": "macOS",
      "device": "desktop",
      "lang": "en-US",
      "is_bot": false
    }
  ],
  "next_page_token": "1714521590000-0"
}`,
        },
        errors: "403 (not owner), 404 (not found)",
      },
      {
        method: "GET",
        path: "/api/v1/urls/:code/analytics",
        title: "Aggregated analytics",
        auth: "owner",
        description: "Return aggregate click metrics derived from recorded click events.",
        params: [
          { name: "stats_type", type: "string", required: true, notes: "day, country, or referer" },
          { name: "from", type: "string", required: false, notes: "RFC 3339 lower bound (inclusive)" },
          { name: "to", type: "string", required: false, notes: "RFC 3339 upper bound (inclusive)" },
          {
            name: "limit",
            type: "integer",
            required: false,
            notes: "Top-N for country and referer (default 50)",
          },
        ],
        curl: `curl --cookie 'urlo_session=…' '${"BASE"}/api/v1/urls/aB3xQ7/analytics?stats_type=referer&limit=5'`,
        response: {
          title: "200 OK",
          body: `{
  "code": "aB3xQ7",
  "stats_type": "referer",
  "items": [
    { "key": "www.google.com", "count": 123 },
    { "key": "(direct)", "count": 42 }
  ]
}`,
        },
        errors: "400 (invalid stats_type / bad from / bad to), 403 (not owner), 404 (not found)",
      },
    ],
  },
  {
    id: "redirect",
    title: "Redirect & health",
    endpoints: [
      {
        method: "GET",
        path: "/:code",
        title: "Redirect",
        description:
          "Public short-link entry point. **Increments `visit_count` by 1** and, if click logging is enabled, asynchronously records a ClickEvent. Reserved paths (`/health`, `/ping`, `/api`) are handled by other routes.",
        curl: `curl -i ${"BASE"}/aB3xQ7`,
        response: {
          title: "302 Found",
          body: `Location: <long_url>`,
        },
        errors: "404 (not found or expired)",
      },
      {
        method: "GET",
        path: "/health",
        title: "Health",
        description: "Liveness probe.",
        curl: `curl ${"BASE"}/health`,
        response: {
          body: `{ "status": "healthy" }`,
        },
      },
    ],
  },
]

function navFrom(gs: Group[]) {
  return gs.map((g) => ({
    id: g.id,
    title: g.title,
    endpoints: g.endpoints.map((ep) => ({ method: ep.method, path: ep.path })),
  }))
}

function DataObjCard({ title, fields }: { title: string; fields: [string, string, string][] }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th className="py-1.5 pr-3 font-medium">Field</th>
              <th className="py-1.5 pr-3 font-medium">Type</th>
              <th className="py-1.5 font-medium">Description</th>
            </tr>
          </thead>
          <tbody>
            {fields.map(([name, type, desc]) => (
              <tr key={name} className="border-b border-border align-top last:border-0">
                <td className="py-1.5 pr-3 font-mono text-foreground">{name}</td>
                <td className="py-1.5 pr-3 font-mono text-muted-foreground">{type}</td>
                <td className="py-1.5 text-muted-foreground">{desc}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  )
}

const statusMapping: [string, string][] = [
  ["InvalidArgument", "400"],
  ["Unauthenticated", "401"],
  ["PermissionDenied", "403"],
  ["NotFound", "404"],
  ["AlreadyExists", "409"],
  ["FailedPrecondition", "412"],
  ["ResourceExhausted", "429"],
  ["Unavailable", "503"],
  ["DeadlineExceeded", "504"],
  ["anything else", "500"],
]

export default function ApiDocs() {
  const base = getApiBaseUrl() || "/"
  const baseField = "BASE"
  const [query, setQuery] = useState("")
  const [methods, setMethods] = useState<ReadonlySet<Method>>(new Set())
  const activeFilter = query.trim() !== "" || methods.size > 0

  const visibleGroups = useMemo(() => {
    const q = query.trim().toLowerCase()
    return groups
      .map((g) => ({
        ...g,
        endpoints: g.endpoints.filter((ep) => {
          if (methods.size > 0 && !methods.has(ep.method)) return false
          if (!q) return true
          return (
            ep.path.toLowerCase().includes(q) ||
            ep.title.toLowerCase().includes(q) ||
            ep.description.toLowerCase().includes(q)
          )
        }),
      }))
      .filter((g) => g.endpoints.length > 0)
  }, [query, methods])

  const nav = navFrom(visibleGroups)

  const toggleMethod = (m: Method) => {
    setMethods((prev) => {
      const next = new Set(prev)
      if (next.has(m)) next.delete(m)
      else next.add(m)
      return next
    })
  }

  const resetFilters = () => {
    setQuery("")
    setMethods(new Set())
  }

  return (
    <div className="space-y-8">
      <div className="space-y-4">
        <div>
          <h1 className="text-3xl font-bold">API Docs</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            HTTP/JSON reference for the urlo URL shortener. This page follows the
            server contract documented in{" "}
            <code className="font-mono text-xs">docs/api/</code>.
          </p>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative sm:max-w-xs sm:flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Filter by path, title…"
              className="pl-9"
            />
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            {allMethods.map((m) => (
              <Button
                key={m}
                size="sm"
                variant={methods.has(m) ? "default" : "outline"}
                onClick={() => toggleMethod(m)}
              >
                {m}
              </Button>
            ))}
            {activeFilter && (
              <Button size="sm" variant="ghost" onClick={resetFilters}>
                <X /> Clear
              </Button>
            )}
          </div>
        </div>

        {/* Mobile group nav */}
        <div className="flex gap-2 overflow-x-auto pb-1 lg:hidden">
          {visibleGroups.map((g) => (
            <a
              key={g.id}
              href={`#${g.id}`}
              className="shrink-0 rounded-full border border-border px-3 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
            >
              {g.title}
            </a>
          ))}
        </div>
      </div>

      <div className="grid gap-8 lg:grid-cols-[220px_1fr]">
        {/* Sidebar */}
        <aside className="hidden lg:block">
          <nav className="sticky top-24 space-y-5 text-sm">
            <div>
              <p className="mb-1.5 px-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Base URL
              </p>
              <div className="rounded-lg border border-border bg-muted/40 px-3 py-2">
                <code className="break-all font-mono text-xs">{base}</code>
              </div>
            </div>
            {nav.map((section) => (
              <div key={section.id}>
                <a
                  href={`#${section.id}`}
                  className="mb-1.5 block px-2 font-medium text-foreground hover:text-primary"
                >
                  {section.title}
                </a>
                <ul className="space-y-0.5 border-l border-border">
                  {section.endpoints.map((ep) => (
                    <li key={ep.path}>
                      <a
                        href={`#${ep.path}`}
                        className="flex items-center gap-1.5 py-0.5 pl-3 text-xs text-muted-foreground hover:text-foreground"
                      >
                        <span
                          className={cn(
                            "w-9 shrink-0 rounded px-1 text-center font-mono text-[10px] font-semibold",
                            methodStyles[ep.method],
                          )}
                        >
                          {ep.method}
                        </span>
                        <span className="truncate font-mono">{ep.path}</span>
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </nav>
        </aside>

        {/* Main */}
        <div className="space-y-10">
          {/* Overview */}
          <section id="overview" className="scroll-mt-24 space-y-4">
            <div className="flex items-center gap-2">
              <Hash className="size-5 text-muted-foreground" />
              <h2 className="text-xl font-bold">Overview</h2>
            </div>
            <Card>
              <CardHeader>
                <CardTitle>Base URL & conventions</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 text-sm text-muted-foreground">
                <p>
                  All endpoints live under the effective base URL. On this page the
                  examples use <code className="font-mono text-xs">{baseField}</code> as a
                  placeholder for the current base URL{" "}
                  <code className="font-mono text-xs">{base}</code>. The content type is{" "}
                  <code className="font-mono text-xs">application/json</code> for all
                  HTTP request/response bodies.
                </p>
                <div className="rounded-lg border border-border bg-muted/40 p-3 font-mono text-xs">
                  GET {base}aB3xQ7&nbsp;&nbsp;→&nbsp;&nbsp;302 to the long URL
                  <br />
                  POST {base}api/v1/urls&nbsp;&nbsp;→&nbsp;&nbsp;create a short link
                </div>
              </CardContent>
            </Card>

            <div className="grid gap-4 md:grid-cols-2">
              <DataObjCard
                title="ShortLink"
                fields={[
                  ["code", "string", "Short code ([A-Za-z0-9], 1–32 chars)"],
                  ["long_url", "string", "Original long URL"],
                  ["short_url", "string", "Fully-qualified short URL"],
                  ["created_at", "string", "RFC 3339 timestamp"],
                  ["expires_at", "string", "RFC 3339; omitted when no expiration"],
                  ["visit_count", "integer", "Total successful resolves"],
                  ["disabled", "boolean", "Present when true; disabled links resolve as 404"],
                ]}
              />
              <DataObjCard
                title="ClickEvent"
                fields={[
                  ["id", "string", "Server-assigned event id"],
                  ["code", "string", "Short code"],
                  ["ts", "string", "RFC 3339 timestamp of the click"],
                  ["ip_hash", "string", "First 16 hex of sha256(ip + salt)"],
                  ["referrer_host", "string", "Lowercase hostname of the referrer"],
                  ["browser", "string", "Chrome / Firefox / Safari / Edge / Opera / Bot / Other"],
                  ["os", "string", "Windows / macOS / iOS / Android / Linux / Other"],
                  ["device", "string", "desktop / mobile / tablet / bot / other"],
                  ["is_bot", "boolean", "True if the UA matches a known bot signature"],
                ]}
              />
            </div>

            <Card>
              <CardHeader>
                <CardTitle>Error format</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 text-sm text-muted-foreground">
                <p>
                  All non-2xx responses use{" "}
                  <code className="font-mono text-xs">
                    {"{ \"error\": \"&lt;grpc_code&gt;\", \"message\": \"&lt;human readable&gt;\" }"}
                  </code>
                  . HTTP status mapping:
                </p>
                <div className="overflow-x-auto rounded-lg border border-border">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-border bg-muted/40 text-left text-muted-foreground">
                        <th className="px-3 py-2 font-medium">grpc_code</th>
                        <th className="px-3 py-2 font-medium">HTTP</th>
                      </tr>
                    </thead>
                    <tbody>
                      {statusMapping.map(([code, http]) => (
                        <tr key={code} className="border-b border-border last:border-0">
                          <td className="px-3 py-1.5 text-muted-foreground">{code}</td>
                          <td className="px-3 py-1.5 font-mono text-foreground">{http}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <p className="text-xs">
                  The rate-limit middleware uses a separate{" "}
                  <code className="font-mono">error: "rate_limited"</code> body and sets a{" "}
                  <code className="font-mono">Retry-After</code> header.
                </p>
              </CardContent>
            </Card>
          </section>

          {/* Groups */}
          {visibleGroups.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border py-16 text-center">
              <p className="text-sm font-medium">No endpoints match your filters</p>
              <Button variant="outline" size="sm" className="mt-3" onClick={resetFilters}>
                Clear filters
              </Button>
            </div>
          ) : (
            visibleGroups.map((group) => (
              <section key={group.id} id={group.id} className="scroll-mt-24 space-y-4">
                <div>
                  <h2 className="text-xl font-bold">{group.title}</h2>
                  {group.description && (
                    <p className="mt-1 text-sm text-muted-foreground">{group.description}</p>
                  )}
                </div>
                {group.endpoints.map((ep) => (
                  <EndpointCard key={ep.path} ep={ep} />
                ))}
              </section>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
