import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { ArrowLeft, RefreshCw } from "lucide-react"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { api, loadLocalLinks, type ShortLink } from "@/lib/api"
import { toast } from "sonner"

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

export default function Analytics() {
  const { code } = useParams<{ code?: string }>()
  const [link, setLink] = useState<ShortLink | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!code) return
    setLoading(true)
    api
      .stats(code)
      .then(setLink)
      .catch((err) =>
        toast.error(err instanceof Error ? err.message : "Failed to load"),
      )
      .finally(() => setLoading(false))
  }, [code])

  if (!code) {
    const links = loadLocalLinks()
    return (
      <div className="space-y-6">
        <h1 className="text-3xl font-bold">Analytics</h1>
        <p className="text-muted-foreground">Pick a link to view its stats.</p>
        <div className="grid gap-3 md:grid-cols-2">
          {links.map((l) => (
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
          {links.length === 0 && (
            <p className="text-muted-foreground">No links yet.</p>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link
            to="/dashboard"
            className={buttonVariants({ variant: "ghost", size: "icon" })}
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1 className="text-3xl font-bold">Link Analytics</h1>
            <p className="text-muted-foreground text-sm font-mono mt-1">
              /{code}
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          disabled={loading}
          onClick={() =>
            code &&
            api
              .stats(code)
              .then(setLink)
              .catch((err) =>
                toast.error(err instanceof Error ? err.message : "Failed"),
              )
          }
        >
          <RefreshCw className="h-4 w-4 mr-2" /> Refresh
        </Button>
      </div>

      {link && (
        <>
          <div className="grid gap-4 md:grid-cols-3">
            <Stat label="Total Clicks" value={link.visit_count} />
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

          <Card>
            <CardHeader>
              <CardTitle>Destination</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div>
                <div className="text-xs uppercase text-muted-foreground tracking-wider mb-1">
                  Short URL
                </div>
                <a
                  href={link.short_url}
                  target="_blank"
                  rel="noreferrer"
                  className="font-mono text-primary hover:underline"
                >
                  {link.short_url}
                </a>
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
            </CardContent>
          </Card>

          <p className="text-sm text-muted-foreground">
            The current backend exposes only aggregate visit counts.
            Per-referrer / device breakdowns will appear here once the API
            supports them.
          </p>
        </>
      )}
    </div>
  )
}
