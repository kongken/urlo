export interface ShortLink {
  code: string
  long_url: string
  short_url: string
  created_at: string
  expires_at?: string
  visit_count: number
}

export interface ApiError {
  error: string
  message: string
}

export interface ShortenRequest {
  long_url: string
  custom_code?: string
  ttl_seconds?: number
  code_length?: number
}

function getBaseUrl(): string {
  const override =
    typeof localStorage !== "undefined"
      ? localStorage.getItem("urlo:apiBaseUrl")
      : null
  if (override) return override
  return (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? ""
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${getBaseUrl()}${path}`, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    const err = data as ApiError | null
    const e = new Error(err?.message || err?.error || `HTTP ${res.status}`) as Error & { status?: number }
    e.status = res.status
    throw e
  }
  return data as T
}

export interface AuthUser {
  sub: string
  email?: string
  name?: string
}

export interface ClickEvent {
  id: string
  code: string
  ts: string
  ip_hash: string
  country: string
  city: string
  referrer: string
  referrer_host: string
  user_agent: string
  browser: string
  os: string
  device: string
  lang: string
  is_bot: boolean
}

export interface ListClicksResponse {
  events: ClickEvent[]
  next_page_token: string
}

export interface AnalyticsItem {
  key: string
  count: number
}

export interface AnalyticsResponse {
  code: string
  stats_type: "day" | "country" | "referer"
  from?: string
  to?: string
  items: AnalyticsItem[]
}

export function getApiBaseUrl(): string {
  return getBaseUrl()
}

export const api = {
  health(baseUrl?: string) {
    const prefix = baseUrl?.trim() ?? getBaseUrl()
    return fetch(`${prefix}/health`, { credentials: "include" }).then(async (res) => {
      const text = await res.text()
      const data = text ? JSON.parse(text) : null
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      return data as { status?: string; message?: string }
    })
  },
  shorten(body: ShortenRequest) {
    return request<ShortLink>("/api/v1/urls", {
      method: "POST",
      body: JSON.stringify(body),
    })
  },
  stats(code: string) {
    return request<ShortLink>(`/api/v1/urls/${encodeURIComponent(code)}/stats`)
  },
  resolve(code: string) {
    return request<ShortLink>(`/api/v1/urls/${encodeURIComponent(code)}`)
  },
  lookup(code: string) {
    return request<ShortLink>(`/api/v1/urls/${encodeURIComponent(code)}/lookup`)
  },
  delete(code: string) {
    return request<void>(`/api/v1/urls/${encodeURIComponent(code)}`, {
      method: "DELETE",
    })
  },
  listMine() {
    return request<{ links: ShortLink[] }>("/api/v1/urls").then((r) => r.links ?? [])
  },
  listClicks(code: string, opts?: { pageSize?: number; pageToken?: string }) {
    const params = new URLSearchParams()
    if (opts?.pageSize) params.set("page_size", String(opts.pageSize))
    if (opts?.pageToken) params.set("page_token", opts.pageToken)
    const qs = params.toString() ? `?${params.toString()}` : ""
    return request<ListClicksResponse>(
      `/api/v1/urls/${encodeURIComponent(code)}/clicks${qs}`,
    ).then((r) => ({
      events: r.events ?? [],
      next_page_token: r.next_page_token ?? "",
    }))
  },
  analytics(
    code: string,
    opts: { statsType: "day" | "country" | "referer"; from?: string; to?: string; limit?: number },
  ) {
    const params = new URLSearchParams()
    params.set("stats_type", opts.statsType)
    if (opts.from) params.set("from", opts.from)
    if (opts.to) params.set("to", opts.to)
    if (opts.limit) params.set("limit", String(opts.limit))
    return request<AnalyticsResponse>(
      `/api/v1/urls/${encodeURIComponent(code)}/analytics?${params.toString()}`,
    ).then((r) => ({ ...r, items: r.items ?? [] }))
  },
  update(
    code: string,
    body: {
      long_url?: string
      ttl_seconds?: number
    },
  ) {
    return request<ShortLink>(`/api/v1/urls/${encodeURIComponent(code)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    })
  },
  setStatus(code: string, body: { disabled: boolean; reason?: string }) {
    return request<{ code: string; disabled: boolean; reason?: string }>(
      `/api/v1/urls/${encodeURIComponent(code)}/status`,
      {
        method: "PATCH",
        body: JSON.stringify(body),
      },
    )
  },
  getStatus(code: string) {
    return request<{ code: string; disabled: boolean; reason?: string }>(
      `/api/v1/urls/${encodeURIComponent(code)}/status`,
    )
  },
  checkAvailability(code: string) {
    return request<{ code: string; available: boolean }>(
      `/api/v1/urls/availability?code=${encodeURIComponent(code)}`,
    )
  },
  // Auth
  loginWithGoogle(idToken: string) {
    return request<{ user: AuthUser }>("/api/v1/auth/google", {
      method: "POST",
      body: JSON.stringify({ id_token: idToken }),
    }).then((r) => r.user)
  },
  logout() {
    return request<void>("/api/v1/auth/logout", { method: "POST" })
  },
  me() {
    return request<{ user: AuthUser }>("/api/v1/auth/me").then((r) => r.user)
  },
}

const STORAGE_KEY = "urlo:links"

export function loadLocalLinks(): ShortLink[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const arr = JSON.parse(raw) as ShortLink[]
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

export function saveLocalLinks(links: ShortLink[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(links))
}

export function upsertLocalLink(link: ShortLink) {
  const links = loadLocalLinks()
  const idx = links.findIndex((l) => l.code === link.code)
  if (idx >= 0) links[idx] = link
  else links.unshift(link)
  saveLocalLinks(links)
}

export function removeLocalLink(code: string) {
  saveLocalLinks(loadLocalLinks().filter((l) => l.code !== code))
}
