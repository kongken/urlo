import { useEffect, useState } from "react"
import { BarChart3, QrCode, Wand2, Copy, Check } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { api, upsertLocalLink, type ShortLink } from "@/lib/api"
import { useAuth } from "@/contexts/AuthContext"
import { QrCard } from "@/components/QrCard"
import { toast } from "sonner"

const features = [
  {
    icon: Wand2,
    title: "Custom Aliases",
    desc: "Pick a memorable code like /launch instead of random characters.",
  },
  {
    icon: QrCode,
    title: "QR Generation",
    desc: "Generate scannable QR codes for any short link, instantly.",
  },
  {
    icon: BarChart3,
    title: "Real-time Analytics",
    desc: "Track every visit. See live counts as people open your links.",
  },
]

type CodeStyle = "short" | "long"

const CODE_LENGTHS: Record<CodeStyle, number> = {
  short: 6,
  long: 12,
}

export default function Landing() {
  const { user } = useAuth()
  const [longUrl, setLongUrl] = useState("")
  const [customCode, setCustomCode] = useState("")
  const [codeStyle, setCodeStyle] = useState<CodeStyle>("short")
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<ShortLink | null>(null)
  const [copied, setCopied] = useState(false)
  const [checkingCode, setCheckingCode] = useState(false)
  const [codeAvailable, setCodeAvailable] = useState<boolean | null>(null)

  useEffect(() => {
    const code = customCode.trim()
    if (!code) {
      setCodeAvailable(null)
      return
    }
    const tid = setTimeout(async () => {
      try {
        setCheckingCode(true)
        const res = await api.checkAvailability(code)
        setCodeAvailable(res.available)
      } catch {
        setCodeAvailable(null)
      } finally {
        setCheckingCode(false)
      }
    }, 300)
    return () => clearTimeout(tid)
  }, [customCode])

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!longUrl.trim()) return
    if (customCode.trim() && codeAvailable === false) {
      toast.error("Custom code is already taken")
      return
    }
    setLoading(true)
    try {
      const trimmedCustom = customCode.trim()
      const link = await api.shorten({
        long_url: longUrl.trim(),
        custom_code: trimmedCustom || undefined,
        // code_length is ignored server-side when custom_code is set,
        // so only include it for auto-generated codes.
        code_length: trimmedCustom ? undefined : CODE_LENGTHS[codeStyle],
      })
      if (!user) upsertLocalLink(link)
      setResult(link)
      toast.success("Short link created")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to shorten")
    } finally {
      setLoading(false)
    }
  }

  async function copy(value: string) {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="space-y-10 md:space-y-16">
      <section className="text-center space-y-6 pt-6 md:pt-10">
        <h1 className="text-3xl font-bold tracking-tight sm:text-5xl md:text-6xl">
          Precision Link Management
        </h1>
        <p className="text-base sm:text-lg text-muted-foreground max-w-xl mx-auto px-2">
          Shorten, share, and track URLs with confidence. Built fast in Go,
          designed for clarity.
        </p>
        <form onSubmit={onSubmit} className="mx-auto max-w-2xl space-y-3 px-1">
          <div className="flex flex-col sm:flex-row gap-2 sm:rounded-full sm:bg-card sm:p-1.5 sm:shadow-sm sm:border">
            <Input
              type="url"
              required
              value={longUrl}
              onChange={(e) => setLongUrl(e.target.value)}
              placeholder="Paste a long URL here…"
              className="sm:border-0 sm:bg-transparent sm:shadow-none sm:focus-visible:ring-0 text-base px-4"
            />
            <Button type="submit" disabled={loading} size="lg" className="rounded-full px-6 w-full sm:w-auto">
              {loading ? "Shortening…" : "Shorten URL"}
            </Button>
          </div>
          <div className="flex flex-col sm:flex-row flex-wrap items-center justify-center gap-2">
            <Input
              value={customCode}
              onChange={(e) => setCustomCode(e.target.value)}
              placeholder="Optional custom code (e.g. launch)"
              className="w-full sm:max-w-xs text-sm"
            />
            {customCode.trim() && (
              <span className="text-xs text-muted-foreground">
                {checkingCode
                  ? "Checking code..."
                  : codeAvailable === null
                    ? "Unable to verify now"
                    : codeAvailable
                      ? "Code is available"
                      : "Code is already taken"}
              </span>
            )}
            <div
              role="radiogroup"
              aria-label="Code length"
              className="inline-flex items-center rounded-full border bg-card p-0.5 text-sm"
            >
              {(["short", "long"] as CodeStyle[]).map((opt) => {
                const active = codeStyle === opt
                return (
                  <button
                    key={opt}
                    type="button"
                    role="radio"
                    aria-checked={active}
                    onClick={() => setCodeStyle(opt)}
                    disabled={!!customCode.trim()}
                    className={
                      "rounded-full px-3 py-1 transition-colors disabled:opacity-50 disabled:cursor-not-allowed " +
                      (active
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:text-foreground")
                    }
                  >
                    {opt === "short"
                      ? `Short (${CODE_LENGTHS.short})`
                      : `Long (${CODE_LENGTHS.long})`}
                  </button>
                )
              })}
            </div>
          </div>
        </form>

        {result && (
          <Card className="mx-auto max-w-2xl text-left">
            <CardHeader>
              <CardTitle className="text-base">Your short link</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="flex items-center gap-2 min-w-0">
                <a
                  href={result.short_url}
                  target="_blank"
                  rel="noreferrer"
                  className="font-mono text-primary hover:underline truncate text-sm sm:text-base"
                >
                  {result.short_url}
                </a>
                <Button size="icon" variant="ghost" className="shrink-0" onClick={() => copy(result.short_url)}>
                  {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                </Button>
              </div>
              <p className="text-sm text-muted-foreground truncate">
                → {result.long_url}
              </p>
              <div className="pt-2">
                <QrCard value={result.short_url} filename={result.code} />
              </div>
            </CardContent>
          </Card>
        )}
      </section>

      <section className="grid gap-6 md:grid-cols-3">
        {features.map((f) => (
          <Card key={f.title}>
            <CardHeader>
              <div className="grid h-10 w-10 place-items-center rounded-lg bg-primary/10 text-primary">
                <f.icon className="h-5 w-5" />
              </div>
              <CardTitle className="mt-3">{f.title}</CardTitle>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              {f.desc}
            </CardContent>
          </Card>
        ))}
      </section>
    </div>
  )
}
