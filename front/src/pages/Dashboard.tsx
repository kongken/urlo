import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { BarChart3, Copy, QrCode, RefreshCw, Trash2, Search, Pencil, Ban, CheckCircle2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { buttonVariants } from "@/components/ui/button-variants"
import { Input } from "@/components/ui/input"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
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
  removeLocalLink,
  upsertLocalLink,
  type ShortLink,
} from "@/lib/api"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { QrCard } from "@/components/QrCard"
import { toast } from "sonner"
import { useAuth } from "@/contexts/useAuth"

function isExpired(link: ShortLink) {
  return Boolean(link.expires_at && new Date(link.expires_at) < new Date())
}

type TTLMode = "keep" | "never" | "1h" | "1d" | "7d" | "30d" | "custom"

const TTL_OPTIONS: { value: TTLMode; label: string; seconds?: number }[] = [
  { value: "keep", label: "Keep current" },
  { value: "never", label: "Never expire", seconds: 0 },
  { value: "1h", label: "1 hour", seconds: 60 * 60 },
  { value: "1d", label: "1 day", seconds: 24 * 60 * 60 },
  { value: "7d", label: "7 days", seconds: 7 * 24 * 60 * 60 },
  { value: "30d", label: "30 days", seconds: 30 * 24 * 60 * 60 },
  { value: "custom", label: "Custom" },
]

function formatExpiry(link: ShortLink) {
  if (!link.expires_at) return "Never expires"
  return `Expires ${new Date(link.expires_at).toLocaleString()}`
}

export default function Dashboard() {
  const { user, loading: authLoading } = useAuth()
  const [links, setLinks] = useState<ShortLink[]>([])
  const [filter, setFilter] = useState("")
  const [qrLink, setQrLink] = useState<ShortLink | null>(null)
  const [disabledMap, setDisabledMap] = useState<Record<string, boolean>>({})
  const [editLink, setEditLink] = useState<ShortLink | null>(null)
  const [editURL, setEditURL] = useState("")
  const [ttlMode, setTTLMode] = useState<TTLMode>("keep")
  const [customTTLSeconds, setCustomTTLSeconds] = useState("")
  const [statusLink, setStatusLink] = useState<ShortLink | null>(null)
  const [statusReason, setStatusReason] = useState("")
  const [savingEdit, setSavingEdit] = useState(false)
  const [savingStatus, setSavingStatus] = useState(false)
  const [selectedCodes, setSelectedCodes] = useState<string[]>([])
  const [bulkBusy, setBulkBusy] = useState(false)

  const loadLinks = async () => {
    if (user) {
      try {
        const list = await api.listMine()
        setLinks(list)
        const statusEntries = await Promise.all(
          list.map(async (l) => {
            try {
              const s = await api.getStatus(l.code)
              return [l.code, s.disabled] as const
            } catch {
              return [l.code, false] as const
            }
          }),
        )
        setDisabledMap(Object.fromEntries(statusEntries))
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to load links")
      }
    } else {
      const local = loadLocalLinks()
      setLinks(local)
      const statusEntries = await Promise.all(
        local.map(async (l) => {
          try {
            const s = await api.getStatus(l.code)
            return [l.code, s.disabled] as const
          } catch {
            return [l.code, false] as const
          }
        }),
      )
      setDisabledMap(Object.fromEntries(statusEntries))
    }
  }

  useEffect(() => {
    if (authLoading) return
    void loadLinks()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authLoading, user?.sub])

  async function refresh(code: string) {
    try {
      const link = await api.stats(code)
      const status = await api.getStatus(code)
      setDisabledMap((prev) => ({ ...prev, [code]: status.disabled }))
      if (user) {
        setLinks((prev) => prev.map((l) => (l.code === code ? link : l)))
      } else {
        upsertLocalLink(link)
        setLinks(loadLocalLinks())
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Refresh failed")
    }
  }

  async function refreshAll() {
    if (user) {
      await loadLinks()
      toast.success("Stats refreshed")
      return
    }
    const current = loadLocalLinks()
    const updated = await Promise.all(
      current.map(async (l) => {
        try {
          return await api.stats(l.code)
        } catch {
          return l
        }
      }),
    )
    updated.forEach(upsertLocalLink)
    setLinks(loadLocalLinks())
    toast.success("Stats refreshed")
  }

  async function onDelete(code: string) {
    if (!confirm(`Delete /${code}?`)) return
    try {
      await api.delete(code)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Delete failed")
      return
    }
    if (user) {
      setLinks((prev) => prev.filter((l) => l.code !== code))
    } else {
      removeLocalLink(code)
      setLinks(loadLocalLinks())
    }
    setSelectedCodes((prev) => prev.filter((c) => c !== code))
    toast.success("Link deleted")
  }

  async function onEditSubmit() {
    if (!editLink) return
    const next = editURL.trim()
    const body: { long_url?: string; ttl_seconds?: number } = {}

    if (next && next !== editLink.long_url) {
      body.long_url = next
    } else if (!next) {
      toast.error("Long URL is required")
      return
    }

    if (ttlMode !== "keep") {
      if (ttlMode === "custom") {
        const seconds = Number(customTTLSeconds)
        if (!Number.isInteger(seconds) || seconds < 0) {
          toast.error("Custom TTL must be a non-negative number of seconds")
          return
        }
        body.ttl_seconds = seconds
      } else {
        body.ttl_seconds = TTL_OPTIONS.find((opt) => opt.value === ttlMode)?.seconds
      }
    }

    if (body.long_url === undefined && body.ttl_seconds === undefined) {
      setEditLink(null)
      return
    }

    setSavingEdit(true)
    try {
      const updated = await api.update(editLink.code, body)
      if (user) {
        setLinks((prev) => prev.map((l) => (l.code === editLink.code ? updated : l)))
      } else {
        upsertLocalLink(updated)
        setLinks(loadLocalLinks())
      }
      setEditLink(null)
      setTTLMode("keep")
      setCustomTTLSeconds("")
      toast.success("Link updated")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Update failed")
    } finally {
      setSavingEdit(false)
    }
  }

  async function onStatusSubmit() {
    if (!statusLink) return
    const currentlyDisabled = !!disabledMap[statusLink.code]
    setSavingStatus(true)
    try {
      const res = await api.setStatus(statusLink.code, {
        disabled: !currentlyDisabled,
        reason: statusReason.trim() || undefined,
      })
      setDisabledMap((prev) => ({ ...prev, [statusLink.code]: res.disabled }))
      setStatusLink(null)
      setStatusReason("")
      toast.success(res.disabled ? "Link disabled" : "Link enabled")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Status update failed")
    } finally {
      setSavingStatus(false)
    }
  }

  const filtered = links.filter(
    (l) =>
      !filter ||
      l.code.toLowerCase().includes(filter.toLowerCase()) ||
      l.long_url.toLowerCase().includes(filter.toLowerCase()),
  )
  const filteredCodes = filtered.map((l) => l.code)
  const selectedSet = new Set(selectedCodes)
  const selectedVisibleCodes = filteredCodes.filter((code) => selectedSet.has(code))
  const allFilteredSelected = filteredCodes.length > 0 && selectedVisibleCodes.length === filteredCodes.length

  function toggleSelected(code: string, checked: boolean) {
    setSelectedCodes((prev) => checked ? [...new Set([...prev, code])] : prev.filter((c) => c !== code))
  }

  function toggleAllFiltered(checked: boolean) {
    setSelectedCodes((prev) => {
      const next = new Set(prev)
      for (const code of filteredCodes) {
        if (checked) next.add(code)
        else next.delete(code)
      }
      return [...next]
    })
  }

  async function bulkRefresh() {
    const codes = selectedCodes
    if (codes.length === 0) return
    setBulkBusy(true)
    let failed = 0
    for (const code of codes) {
      try {
        const link = await api.stats(code)
        const status = await api.getStatus(code)
        setDisabledMap((prev) => ({ ...prev, [code]: status.disabled }))
        if (user) {
          setLinks((prev) => prev.map((l) => (l.code === code ? link : l)))
        } else {
          upsertLocalLink(link)
        }
      } catch {
        failed++
      }
    }
    if (!user) setLinks(loadLocalLinks())
    setBulkBusy(false)
    toast[failed ? "warning" : "success"](
      failed ? `Refreshed ${codes.length - failed}; ${failed} failed` : `Refreshed ${codes.length} links`,
    )
  }

  async function bulkSetStatus(disabled: boolean) {
    const codes = selectedCodes
    if (codes.length === 0) return
    setBulkBusy(true)
    let ok = 0
    let failed = 0
    for (const code of codes) {
      try {
        const res = await api.setStatus(code, { disabled })
        setDisabledMap((prev) => ({ ...prev, [code]: res.disabled }))
        ok++
      } catch {
        failed++
      }
    }
    setBulkBusy(false)
    toast[failed ? "warning" : "success"](
      failed ? `${ok} updated; ${failed} failed` : `${ok} links ${disabled ? "disabled" : "enabled"}`,
    )
  }

  async function bulkDelete() {
    const codes = selectedCodes
    if (codes.length === 0) return
    if (!confirm(`Delete ${codes.length} selected link${codes.length === 1 ? "" : "s"}?`)) return
    setBulkBusy(true)
    let ok = 0
    let failed = 0
    for (const code of codes) {
      try {
        await api.delete(code)
        if (!user) removeLocalLink(code)
        ok++
      } catch {
        failed++
      }
    }
    if (user) {
      setLinks((prev) => prev.filter((l) => !codes.includes(l.code)))
    } else {
      setLinks(loadLocalLinks())
    }
    setSelectedCodes((prev) => prev.filter((code) => !codes.includes(code)))
    setBulkBusy(false)
    toast[failed ? "warning" : "success"](
      failed ? `${ok} deleted; ${failed} failed` : `${ok} links deleted`,
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold">My Links</h1>
          <p className="text-muted-foreground text-sm mt-1">
            {user
              ? `Signed in as ${user.email || user.name || user.sub}.`
              : "Stored locally in this browser. Sign in to manage links across devices."}
          </p>
        </div>
        <Button onClick={refreshAll} variant="outline" className="self-start sm:self-auto">
          <RefreshCw className="h-4 w-4 mr-2" /> Refresh stats
        </Button>
      </div>

      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div className="relative max-w-md flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Search links…"
            className="pl-9"
          />
        </div>
        {selectedCodes.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-card p-2 text-sm">
            <span className="text-muted-foreground">{selectedCodes.length} selected</span>
            <Button size="sm" variant="outline" disabled={bulkBusy} onClick={bulkRefresh}>
              <RefreshCw className="h-3.5 w-3.5 mr-1" /> Refresh
            </Button>
            <Button size="sm" variant="outline" disabled={bulkBusy} onClick={() => bulkSetStatus(false)}>
              <CheckCircle2 className="h-3.5 w-3.5 mr-1" /> Enable
            </Button>
            <Button size="sm" variant="outline" disabled={bulkBusy} onClick={() => bulkSetStatus(true)}>
              <Ban className="h-3.5 w-3.5 mr-1" /> Disable
            </Button>
            <Button size="sm" variant="destructive" disabled={bulkBusy} onClick={bulkDelete}>
              <Trash2 className="h-3.5 w-3.5 mr-1" /> Delete
            </Button>
          </div>
        )}
      </div>

      <Card>
        {filtered.length === 0 ? (
          <div className="p-12 text-center text-muted-foreground">
            <p>No links yet.</p>
            <Link to="/" className="text-primary hover:underline">
              Create your first short link →
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[1%]">
                    <input
                      type="checkbox"
                      aria-label="Select all filtered links"
                      checked={allFilteredSelected}
                      onChange={(e) => toggleAllFiltered(e.currentTarget.checked)}
                      className="h-4 w-4 rounded border-border accent-primary"
                    />
                  </TableHead>
                  <TableHead>Short URL</TableHead>
                  <TableHead className="hidden sm:table-cell">Destination</TableHead>
                  <TableHead className="text-right">Clicks</TableHead>
                  <TableHead className="hidden sm:table-cell">Status</TableHead>
                  <TableHead className="w-[1%]"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((link) => {
                  const expired = isExpired(link)
                  const disabled = !!disabledMap[link.code]
                  return (
                    <TableRow key={link.code} className={expired || disabled ? "opacity-60" : ""}>
                      <TableCell>
                        <input
                          type="checkbox"
                          aria-label={`Select ${link.code}`}
                          checked={selectedSet.has(link.code)}
                          onChange={(e) => toggleSelected(link.code, e.currentTarget.checked)}
                          className="h-4 w-4 rounded border-border accent-primary"
                        />
                      </TableCell>
                      <TableCell className="font-mono text-xs sm:text-sm">
                        <a
                          href={link.short_url}
                          target="_blank"
                          rel="noreferrer"
                          className="text-primary hover:underline"
                        >
                          {link.short_url.replace(/^https?:\/\//, "")}
                        </a>
                      </TableCell>
                      <TableCell className="max-w-xs text-muted-foreground hidden sm:table-cell">
                        <div className="truncate">{link.long_url}</div>
                        <div className="mt-1 text-xs">{formatExpiry(link)}</div>
                      </TableCell>
                      <TableCell className="text-right font-mono">
                        {link.visit_count}
                      </TableCell>
                      <TableCell className="hidden sm:table-cell">
                        {expired ? (
                          <Badge variant="outline">Expired</Badge>
                        ) : disabled ? (
                          <Badge variant="secondary">Disabled</Badge>
                        ) : (
                          <Badge>Active</Badge>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-1">
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => {
                              navigator.clipboard.writeText(link.short_url)
                              toast.success("Copied")
                            }}
                            title="Copy"
                          >
                            <Copy className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => setQrLink(link)}
                            title="QR code"
                          >
                            <QrCode className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => refresh(link.code)}
                            title="Refresh stats"
                          >
                            <RefreshCw className="h-4 w-4" />
                          </Button>
                          <Link
                            to={`/analytics/${link.code}`}
                            title="Analytics"
                            className={buttonVariants({ variant: "ghost", size: "icon" })}
                          >
                            <BarChart3 className="h-4 w-4" />
                          </Link>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => {
                              setEditLink(link)
                              setEditURL(link.long_url)
                              setTTLMode("keep")
                              setCustomTTLSeconds("")
                            }}
                            title="Edit destination and expiration"
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => {
                              setStatusLink(link)
                              setStatusReason("")
                            }}
                            title={disabled ? "Enable" : "Disable"}
                          >
                            {disabled ? <CheckCircle2 className="h-4 w-4" /> : <Ban className="h-4 w-4" />}
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => onDelete(link.code)}
                            title="Delete"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </Card>

      <Dialog open={!!qrLink} onOpenChange={(open) => !open && setQrLink(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle className="font-mono text-base">
              {qrLink?.short_url.replace(/^https?:\/\//, "")}
            </DialogTitle>
          </DialogHeader>
          {qrLink && (
            <div className="flex flex-col items-center gap-4">
              <QrCard
                value={qrLink.short_url}
                filename={qrLink.code}
                size={220}
              />
            </div>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={!!editLink} onOpenChange={(open) => !open && setEditLink(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit destination</DialogTitle>
            <DialogDescription className="font-mono text-xs">/{editLink?.code}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm text-muted-foreground" htmlFor="edit-destination">
                Long URL
              </label>
              <Input
                id="edit-destination"
                type="url"
                value={editURL}
                onChange={(e) => setEditURL(e.target.value)}
                placeholder="https://example.com/new-target"
              />
            </div>
            <div className="space-y-2">
              <div className="flex flex-col gap-1">
                <label className="text-sm text-muted-foreground">Expiration</label>
                <span className="text-xs text-muted-foreground">
                  Current: {editLink ? formatExpiry(editLink) : "—"}
                </span>
              </div>
              <div className="flex flex-wrap gap-2">
                {TTL_OPTIONS.map((opt) => (
                  <Button
                    key={opt.value}
                    type="button"
                    size="sm"
                    variant={ttlMode === opt.value ? "default" : "outline"}
                    onClick={() => setTTLMode(opt.value)}
                    disabled={savingEdit}
                  >
                    {opt.label}
                  </Button>
                ))}
              </div>
              {ttlMode === "custom" && (
                <div className="space-y-1">
                  <Input
                    type="number"
                    min={0}
                    step={1}
                    inputMode="numeric"
                    value={customTTLSeconds}
                    onChange={(e) => setCustomTTLSeconds(e.target.value)}
                    placeholder="TTL in seconds; 0 means never expire"
                  />
                  <p className="text-xs text-muted-foreground">
                    The expiration is recalculated from the time you save.
                  </p>
                </div>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditLink(null)} disabled={savingEdit}>
              Cancel
            </Button>
            <Button onClick={onEditSubmit} disabled={savingEdit}>
              {savingEdit ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!statusLink} onOpenChange={(open) => !open && setStatusLink(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{statusLink && disabledMap[statusLink.code] ? "Enable link" : "Disable link"}</DialogTitle>
            <DialogDescription className="font-mono text-xs">/{statusLink?.code}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <label className="text-sm text-muted-foreground" htmlFor="status-reason">
              Reason (optional)
            </label>
            <Input
              id="status-reason"
              value={statusReason}
              onChange={(e) => setStatusReason(e.target.value)}
              placeholder="abuse / maintenance / etc."
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setStatusLink(null)} disabled={savingStatus}>
              Cancel
            </Button>
            <Button onClick={onStatusSubmit} disabled={savingStatus}>
              {savingStatus
                ? "Saving..."
                : statusLink && disabledMap[statusLink.code]
                  ? "Enable"
                  : "Disable"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
