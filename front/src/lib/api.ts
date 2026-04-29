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
    throw new Error(err?.message || err?.error || `HTTP ${res.status}`)
  }
  return data as T
}

export const api = {
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
  delete(code: string) {
    return request<void>(`/api/v1/urls/${encodeURIComponent(code)}`, {
      method: "DELETE",
    })
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
