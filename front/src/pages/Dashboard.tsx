import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { BarChart3, Copy, RefreshCw, Trash2, Search } from "lucide-react"
import { Button, buttonVariants } from "@/components/ui/button"
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
import { toast } from "sonner"
import { useAuth } from "@/contexts/AuthContext"

function isExpired(link: ShortLink) {
  return Boolean(link.expires_at && new Date(link.expires_at) < new Date())
}

export default function Dashboard() {
  const { user, loading: authLoading } = useAuth()
  const [links, setLinks] = useState<ShortLink[]>([])
  const [filter, setFilter] = useState("")

  const loadLinks = async () => {
    if (user) {
      try {
        const list = await api.listMine()
        setLinks(list)
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to load links")
      }
    } else {
      setLinks(loadLocalLinks())
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
    toast.success("Link deleted")
  }

  const filtered = links.filter(
    (l) =>
      !filter ||
      l.code.toLowerCase().includes(filter.toLowerCase()) ||
      l.long_url.toLowerCase().includes(filter.toLowerCase()),
  )

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">My Links</h1>
          <p className="text-muted-foreground text-sm mt-1">
            {user
              ? `Signed in as ${user.email || user.name || user.sub}.`
              : "Stored locally in this browser. Sign in to manage links across devices."}
          </p>
        </div>
        <Button onClick={refreshAll} variant="outline">
          <RefreshCw className="h-4 w-4 mr-2" /> Refresh stats
        </Button>
      </div>

      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Search links…"
          className="pl-9"
        />
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
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Short URL</TableHead>
                <TableHead>Destination</TableHead>
                <TableHead className="text-right">Clicks</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-[1%]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((link) => {
                const expired = isExpired(link)
                return (
                  <TableRow key={link.code} className={expired ? "opacity-60" : ""}>
                    <TableCell className="font-mono">
                      <a
                        href={link.short_url}
                        target="_blank"
                        rel="noreferrer"
                        className="text-primary hover:underline"
                      >
                        {link.short_url.replace(/^https?:\/\//, "")}
                      </a>
                    </TableCell>
                    <TableCell className="max-w-xs truncate text-muted-foreground">
                      {link.long_url}
                    </TableCell>
                    <TableCell className="text-right font-mono">
                      {link.visit_count}
                    </TableCell>
                    <TableCell>
                      {expired ? (
                        <Badge variant="outline">Expired</Badge>
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
        )}
      </Card>
    </div>
  )
}
