import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { ArrowLeft, RefreshCw } from "lucide-react"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  api,
  loadLocalLinks,
  type AnalyticsItem,
  type ClickEvent,
  type ShortLink,
} from "@/lib/api"
import { useAuth } from "@/contexts/AuthContext"
import { QrCard } from "@/components/QrCard"
import { toast } from "sonner"

const FETCH_PAGE_SIZE = 500

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-xs uppercase tracking-wider text-muted-foreground">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold">{value}</div>
      </CardContent>
    </Card>
  )
}

function topNFromEvents(
  events: ClickEvent[],
  pick: (e: ClickEvent) => string,
  n = 5,
  fallback = "(unknown)",
) {
  const counts = new Map<string, number>()
  for (const e of events) {
    const key = (pick(e) || "").trim() || fallback
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, n)
    .map(([label, count]) => ({ label, count }))
}

function Breakdown({
  title,
  rows,
  total,
  empty = "No data yet.",
}: {
  title: string
  rows: { label: string; count: number }[]
  total: number
  empty?: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        {rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">{empty}</p>
        ) : (
          rows.map((r) => {
            const pct = total > 0 ? Math.round((r.count / total) * 100) : 0
            return (
              <div key={r.label} className="space-y-1">
                <div className="flex justify-between text-sm">
                  <span className="truncate pr-2">{r.label}</span>
                  <span className="text-muted-foreground tabular-nums">
                    {r.count} · {pct}%
                  </span>
                </div>
                <div className="h-1.5 rounded-full bg-muted overflow-hidden">
                  <div
                    className="h-full bg-primary"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            )
          })
        )}
      </CardContent>
    </Card>
  )
}

export default function Analytics() {
  const { code } = useParams<{ code?: string }>()
  const { user } = useAuth()
  const [link, setLink] = useState<ShortLink | null>(null)
  const [events, setEvents] = useState<ClickEvent[]>([])
  const [pickerLinks, setPickerLinks] = useState<ShortLink[]>([])
  const [loading, setLoading] = useState(false)
  const [clicksUnsupported, setClicksUnsupported] = useState(false)
  const [referrers, setReferrers] = useState<AnalyticsItem[]>([])
  const [countries, setCountries] = useState<AnalyticsItem[]>([])
  const [days, setDays] = useState<AnalyticsItem[]>([])
  const [linkDisabled, setLinkDisabled] = useState(false)
  const [linkDisabledReason, setLinkDisabledReason] = useState("")

  useEffect(() => {
    if (code) return
    let cancelled = false
    const load = async () => {
      if (user) {
        try {
          const list = await api.listMine()
          if (!cancelled) setPickerLinks(list)
        } catch {
          if (!cancelled) setPickerLinks([])
        }
      } else {
        setPickerLinks(loadLocalLinks())
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [code, user])

  const refresh = async () => {
    if (!code) return
    setLoading(true)
    setClicksUnsupported(false)
    try {
      const [s, c, st] = await Promise.allSettled([
        api.stats(code),
        api.listClicks(code, { pageSize: FETCH_PAGE_SIZE }),
        api.getStatus(code),
      ])
      if (s.status === "fulfilled") setLink(s.value)
      else
        toast.error(
          s.reason instanceof Error ? s.reason.message : "Failed to load stats",
        )

      if (c.status === "fulfilled") {
        setEvents(c.value.events)
      } else {
        const err = c.reason as Error & { status?: number }
        // 403 = ownership mismatch (anon viewing owned link); show a softer note
        if (err?.status === 403) {
          setClicksUnsupported(true)
          setEvents([])
        } else {
          toast.error(err?.message || "Failed to load clicks")
          setEvents([])
        }
      }
      if (st.status === "fulfilled") {
        setLinkDisabled(st.value.disabled)
        setLinkDisabledReason(st.value.reason || "")
      } else {
        setLinkDisabled(false)
        setLinkDisabledReason("")
      }
      const [aRef, aCountry, aDay] = await Promise.allSettled([
        api.analytics(code, { statsType: "referer", limit: 5 }),
        api.analytics(code, { statsType: "country", limit: 5 }),
        api.analytics(code, { statsType: "day", limit: 14 }),
      ])
      if (aRef.status === "fulfilled") setReferrers(aRef.value.items)
      if (aCountry.status === "fulfilled") setCountries(aCountry.value.items)
      if (aDay.status === "fulfilled") setDays(aDay.value.items)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [code])

  const total = events.length
  const uniqueVisitors = new Set(
    events.map((e) => e.ip_hash).filter(Boolean),
  ).size
  const devices = topNFromEvents(events, (e) => e.device, 5)
  const browsers = topNFromEvents(events, (e) => e.browser, 5)

  if (!code) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl sm:text-3xl font-bold">Analytics</h1>
        <p className="text-muted-foreground">Pick a link to view its stats.</p>
        <div className="grid gap-3 sm:grid-cols-2">
          {pickerLinks.map((l) => (
            <Link
              key={l.code}
              to={`/analytics/${l.code}`}
              className="block rounded-lg border p-4 hover:border-primary transition-colors"
            >
              <div className="font-mono text-primary">{l.code}</div>
              <div className="text-sm text-muted-foreground truncate">
                {l.long_url}
              </div>
              <div className="text-xs mt-1">{l.visit_count} clicks</div>
            </Link>
          ))}
          {pickerLinks.length === 0 && (
            <p className="text-muted-foreground">No links yet.</p>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link
            to="/dashboard"
            className={buttonVariants({ variant: "ghost", size: "icon" })}
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1 className="text-2xl sm:text-3xl font-bold">Link Analytics</h1>
            <p className="text-muted-foreground text-sm font-mono mt-1 break-all">
              /{code}
            </p>
          </div>
        </div>
        <Button variant="outline" disabled={loading} onClick={refresh} className="self-start sm:self-auto">
          <RefreshCw className="h-4 w-4 mr-2" /> Refresh
        </Button>
      </div>

      {link && (
        <div className="grid grid-cols-2 gap-3 sm:gap-4 md:grid-cols-4">
          <Stat label="Total Clicks" value={link.visit_count} />
          <Stat
            label="Unique Visitors"
            value={uniqueVisitors || "—"}
          />
          <Stat
            label="Created"
            value={new Date(link.created_at).toLocaleDateString()}
          />
          <Stat
            label="Expires"
            value={
              link.expires_at
                ? new Date(link.expires_at).toLocaleDateString()
                : "Never"
            }
          />
        </div>
      )}

      {link && (
        <Card>
          <CardHeader>
            <CardTitle>Destination</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {linkDisabled && (
              <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
                This link is currently disabled and cannot be opened via short URL.
                {linkDisabledReason ? ` Reason: ${linkDisabledReason}.` : ""}
              </div>
            )}
            <div>
              <div className="text-xs uppercase text-muted-foreground tracking-wider mb-1">
                Short URL
              </div>
              {linkDisabled ? (
                <span className="font-mono text-muted-foreground">{link.short_url}</span>
              ) : (
                <a
                  href={link.short_url}
                  target="_blank"
                  rel="noreferrer"
                  className="font-mono text-primary hover:underline"
                >
                  {link.short_url}
                </a>
              )}
            </div>
            <div>
              <div className="text-xs uppercase text-muted-foreground tracking-wider mb-1">
                Long URL
              </div>
              <a
                href={link.long_url}
                target="_blank"
                rel="noreferrer"
                className="break-all hover:underline"
              >
                {link.long_url}
              </a>
            </div>
            <div className="pt-2">
              <QrCard value={link.short_url} filename={link.code} />
            </div>
          </CardContent>
        </Card>
      )}

      {clicksUnsupported ? (
        <p className="text-sm text-muted-foreground">
          Detailed click breakdowns require ownership of this link. Sign in
          as the owner to view referrers, devices, and locations.
        </p>
      ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2">
            <Breakdown
              title="Top Referrers"
              rows={referrers.map((i) => ({ label: i.key, count: i.count }))}
              total={total}
            />
            <Breakdown
              title="Locations"
              rows={countries.map((i) => ({ label: i.key, count: i.count }))}
              total={total}
              empty="GeoIP not configured on the server."
            />
            <Breakdown
              title="Daily Trend"
              rows={days.map((i) => ({ label: i.key, count: i.count }))}
              total={Math.max(1, days.reduce((acc, i) => acc + i.count, 0))}
            />
            <Breakdown
              title="Browsers"
              rows={browsers}
              total={total}
            />
            <Breakdown
              title="Devices"
              rows={devices}
              total={total}
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Recent Clicks</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              {events.length === 0 ? (
                <p className="p-6 text-sm text-muted-foreground">
                  No clicks recorded yet. Share your short link to start
                  collecting data.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Time</TableHead>
                        <TableHead>Referrer</TableHead>
                        <TableHead className="hidden sm:table-cell">Browser</TableHead>
                        <TableHead className="hidden sm:table-cell">OS</TableHead>
                        <TableHead className="hidden md:table-cell">Device</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {events.slice(0, 50).map((e) => (
                        <TableRow key={e.id}>
                          <TableCell className="font-mono text-xs whitespace-nowrap">
                            {new Date(e.ts).toLocaleString()}
                          </TableCell>
                          <TableCell className="max-w-[10rem] sm:max-w-[16rem] truncate">
                            {e.referrer_host || (
                              <span className="text-muted-foreground">
                                (direct)
                              </span>
                            )}
                          </TableCell>
                          <TableCell className="hidden sm:table-cell">{e.browser || "—"}</TableCell>
                          <TableCell className="hidden sm:table-cell">{e.os || "—"}</TableCell>
                          <TableCell className="hidden md:table-cell">{e.device || "—"}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>

          {events.length === FETCH_PAGE_SIZE && (
            <p className="text-xs text-muted-foreground">
              Showing the {FETCH_PAGE_SIZE} most recent clicks. Older events
              are not included in these aggregates.
            </p>
          )}
        </>
      )}
    </div>
  )
}
