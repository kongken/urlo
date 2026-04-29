import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { saveLocalLinks, loadLocalLinks } from "@/lib/api"
import { toast } from "sonner"

const OVERRIDE_KEY = "urlo:apiBaseUrl"

export default function Settings() {
  const [baseUrl, setBaseUrl] = useState(
    () => localStorage.getItem(OVERRIDE_KEY) ?? "",
  )

  function save() {
    if (baseUrl.trim()) {
      localStorage.setItem(OVERRIDE_KEY, baseUrl.trim())
    } else {
      localStorage.removeItem(OVERRIDE_KEY)
    }
    toast.success("Saved. Reload the page to apply.")
  }

  function clearLinks() {
    if (!confirm("Clear all locally stored links from this browser?")) return
    saveLocalLinks([])
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
            onChange={(e) => setBaseUrl(e.target.value)}
            placeholder="http://localhost:8080"
          />
          <Button onClick={save}>Save</Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Local Data</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">
            {loadLocalLinks().length} links stored in this browser.
          </p>
          <Button variant="destructive" onClick={clearLinks}>
            Clear local links
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
