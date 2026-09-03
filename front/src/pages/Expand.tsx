import { useState } from "react"
import type { FormEvent } from "react"
import { ArrowRight, Check, Copy, ExternalLink, Loader2, ShieldAlert } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { api, type ExpandResult, type URLRedirect } from "@/lib/api"
import { toast } from "sonner"

function statusIsSuccessful(statusCode: number) {
  return statusCode >= 200 && statusCode < 400
}

function RedirectStep({ redirect, index }: { redirect: URLRedirect; index: number }) {
  return (
    <li className="flex gap-3">
      <span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold">
        {index + 1}
      </span>
      <div className="min-w-0 flex-1 space-y-2">
        <p className="break-all font-mono text-xs text-muted-foreground">{redirect.url}</p>
        <div className="flex min-w-0 items-start gap-2 text-sm">
          <Badge variant="outline" className="shrink-0 font-mono">
            {redirect.status_code}
          </Badge>
          <ArrowRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
          <p className="min-w-0 break-all font-mono text-xs text-foreground">
            {redirect.location}
          </p>
        </div>
      </div>
    </li>
  )
}

function ExpansionResult({ result }: { result: ExpandResult }) {
  const [copied, setCopied] = useState(false)

  async function copyFinalURL() {
    try {
      await navigator.clipboard.writeText(result.final_url)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error("Copy failed")
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <CardTitle className="text-base">Resolved destination</CardTitle>
          <div className="flex items-center gap-2">
            <Badge
              variant={statusIsSuccessful(result.status_code) ? "default" : "destructive"}
              className="font-mono"
            >
              {result.status_code}
            </Badge>
            <Badge variant="outline">
              {result.redirect_count} {result.redirect_count === 1 ? "redirect" : "redirects"}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="flex min-w-0 items-start gap-2 rounded-md border bg-muted/30 p-3">
          <a
            href={result.final_url}
            target="_blank"
            rel="noreferrer"
            className="min-w-0 flex-1 break-all font-mono text-sm text-primary hover:underline"
          >
            {result.final_url}
          </a>
          <Button
            size="icon"
            variant="ghost"
            onClick={copyFinalURL}
            aria-label="Copy final URL"
            title="Copy final URL"
            className="shrink-0"
          >
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
          </Button>
          <a
            href={result.final_url}
            target="_blank"
            rel="noreferrer"
            aria-label="Open final URL"
            title="Open final URL"
            className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <ExternalLink className="h-4 w-4" />
          </a>
        </div>

        {result.redirects.length > 0 && (
          <section className="space-y-3">
            <h2 className="text-sm font-semibold">Redirect chain</h2>
            <ol className="space-y-4">
              {result.redirects.map((redirect, index) => (
                <RedirectStep key={`${redirect.url}-${index}`} redirect={redirect} index={index} />
              ))}
            </ol>
          </section>
        )}
      </CardContent>
    </Card>
  )
}

export default function Expand() {
  const [url, setURL] = useState("")
  const [result, setResult] = useState<ExpandResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const value = url.trim()
    if (!value) return

    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const expanded = await api.expand(value)
      setResult(expanded)
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unable to restore URL"
      setError(message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="mx-auto max-w-4xl space-y-8">
      <div>
        <h1 className="text-3xl font-bold">URL Restore</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Follow public HTTP redirects and inspect the final destination.
        </p>
      </div>

      <Card>
        <CardContent className="pt-6">
          <form onSubmit={onSubmit} className="space-y-3">
            <div className="flex flex-col gap-2 sm:flex-row">
              <Input
                type="url"
                required
                value={url}
                onChange={(event) => setURL(event.target.value)}
                placeholder="https://short.example/link"
                aria-label="Third-party URL"
                autoComplete="url"
                className="min-w-0 flex-1 text-base"
              />
              <Button type="submit" disabled={loading} className="sm:min-w-36">
                {loading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <ArrowRight className="h-4 w-4" />
                )}
                {loading ? "Restoring..." : "Restore URL"}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              HTTP and HTTPS links only. The result is not saved to your account or browser.
            </p>
          </form>
        </CardContent>
      </Card>

      {error && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive"
        >
          <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
          <span className="break-words">{error}</span>
        </div>
      )}

      {result ? (
        <ExpansionResult result={result} />
      ) : (
        <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center text-sm text-muted-foreground">
          The resolved destination will appear here.
        </div>
      )}
    </div>
  )
}
