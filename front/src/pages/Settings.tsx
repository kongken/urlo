import { useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { api, getApiBaseUrl, saveLocalLinks, loadLocalLinks, type ShortLink } from "@/lib/api"
import { toast } from "sonner"

const OVERRIDE_KEY = "urlo:apiBaseUrl"

export default function Settings() {
  const [baseUrl, setBaseUrl] = useState(
    () => localStorage.getItem(OVERRIDE_KEY) ?? "",
  )
  const [localLinks, setLocalLinks] = useState(() => loadLocalLinks())
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<"ok" | "error" | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  const effectiveBaseUrl = baseUrl.trim() || getApiBaseUrl() || "/"

  function save() {
    if (baseUrl.trim()) {
      localStorage.setItem(OVERRIDE_KEY, baseUrl.trim())
    } else {
      localStorage.removeItem(OVERRIDE_KEY)
    }
    toast.success("Saved. Reload the page to apply.")
  }

  async function testConnection() {
    setTesting(true)
    setTestResult(null)
    try {
      const base = baseUrl.trim() || undefined
      const res = await api.health(base)
      if (res.status && res.status !== "healthy") {
        throw new Error(`Unexpected health status: ${res.status}`)
      }
      setTestResult("ok")
      toast.success("API connection healthy")
    } catch (err) {
      setTestResult("error")
      toast.error(err instanceof Error ? err.message : "API connection failed")
    } finally {
      setTesting(false)
    }
  }

  function exportLinks() {
    const links = loadLocalLinks()
    const blob = new Blob([JSON.stringify(links, null, 2)], {
      type: "application/json",
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = `urlo-local-links-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  async function importLinks(file: File | undefined) {
    if (!file) return
    try {
      const raw = await file.text()
      const parsed = JSON.parse(raw) as ShortLink[]
      if (!Array.isArray(parsed)) throw new Error("Expected a JSON array")
      const existing = loadLocalLinks()
      const merged = new Map(existing.map((link) => [link.code, link]))
      for (const link of parsed) {
        if (!link?.code || !link.long_url || !link.short_url) {
          throw new Error("Invalid local link export format")
        }
        merged.set(link.code, link)
      }
      const next = [...merged.values()].sort(
        (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      )
      saveLocalLinks(next)
      setLocalLinks(next)
      toast.success(`Imported ${parsed.length} links`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Import failed")
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = ""
    }
  }

  function clearLinks() {
    if (!confirm("Clear all locally stored links from this browser?")) return
    saveLocalLinks([])
    setLocalLinks([])
    toast.success("Local links cleared")
  }

  return (
    <div className="space-y-6 max-w-2xl">
      <div>
        <h1 className="text-3xl font-bold">Settings</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Local-only preferences for this browser.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>API Base URL</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">
            Override the default backend (build-time:{" "}
            <code className="font-mono text-xs">VITE_API_BASE_URL</code>).
            Leave blank to use the default.
          </p>
          <Input
            value={baseUrl}
            onChange={(e) => {
              setBaseUrl(e.target.value)
              setTestResult(null)
            }}
            placeholder="/ (same origin)"
          />
          <p className="text-xs text-muted-foreground">
            Effective endpoint: <span className="font-mono">{effectiveBaseUrl}</span>
          </p>
          {testResult && (
            <p className={testResult === "ok" ? "text-sm text-emerald-600" : "text-sm text-destructive"}>
              {testResult === "ok" ? "Connection healthy" : "Connection failed"}
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            <Button onClick={save}>Save</Button>
            <Button variant="outline" disabled={testing} onClick={testConnection}>
              {testing ? "Testing..." : "Test connection"}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Local Data</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">
            {localLinks.length} links stored in this browser.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={exportLinks} disabled={localLinks.length === 0}>
              Export links
            </Button>
            <Button variant="outline" onClick={() => fileInputRef.current?.click()}>
              Import links
            </Button>
            <Button variant="destructive" onClick={clearLinks} disabled={localLinks.length === 0}>
              Clear local links
            </Button>
          </div>
          <input
            ref={fileInputRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={(e) => void importLinks(e.currentTarget.files?.[0])}
          />
          <p className="text-xs text-muted-foreground">
            Import merges by short code and keeps newer duplicate records from the imported file.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
