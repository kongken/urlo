import { useEffect, useRef } from "react"
import { toast } from "sonner"
import { useAuth } from "@/contexts/AuthContext"

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize: (cfg: {
            client_id: string
            callback: (resp: { credential?: string }) => void
            auto_select?: boolean
          }) => void
          renderButton: (
            parent: HTMLElement,
            opts: {
              type?: "standard" | "icon"
              theme?: "outline" | "filled_blue" | "filled_black"
              size?: "small" | "medium" | "large"
              text?: "signin_with" | "signup_with" | "continue_with" | "signin"
              shape?: "rectangular" | "pill" | "circle" | "square"
              logo_alignment?: "left" | "center"
            },
          ) => void
          disableAutoSelect: () => void
        }
      }
    }
  }
}

const GSI_SRC = "https://accounts.google.com/gsi/client"

let gsiPromise: Promise<void> | null = null
function loadGsi(): Promise<void> {
  if (gsiPromise) return gsiPromise
  gsiPromise = new Promise((resolve, reject) => {
    if (window.google?.accounts?.id) return resolve()
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${GSI_SRC}"]`)
    if (existing) {
      existing.addEventListener("load", () => resolve())
      existing.addEventListener("error", () => reject(new Error("failed to load GSI")))
      return
    }
    const s = document.createElement("script")
    s.src = GSI_SRC
    s.async = true
    s.defer = true
    s.onload = () => resolve()
    s.onerror = () => reject(new Error("failed to load GSI"))
    document.head.appendChild(s)
  })
  return gsiPromise
}

export function LoginButton() {
  const { loginWithGoogle } = useAuth()
  const ref = useRef<HTMLDivElement>(null)
  const clientId = import.meta.env.VITE_GOOGLE_CLIENT_ID as string | undefined

  useEffect(() => {
    if (!clientId || !ref.current) return
    let cancelled = false
    void loadGsi()
      .then(() => {
        if (cancelled || !ref.current || !window.google) return
        window.google.accounts.id.initialize({
          client_id: clientId,
          callback: async (resp) => {
            if (!resp.credential) return
            try {
              await loginWithGoogle(resp.credential)
              toast.success("Signed in")
            } catch (e) {
              toast.error(e instanceof Error ? e.message : "login failed")
            }
          },
        })
        window.google.accounts.id.renderButton(ref.current, {
          type: "standard",
          theme: "outline",
          size: "medium",
          text: "signin_with",
          shape: "pill",
        })
      })
      .catch(() => toast.error("Could not load Google sign-in"))
    return () => {
      cancelled = true
    }
  }, [clientId, loginWithGoogle])

  if (!clientId) {
    return (
      <span className="text-xs text-muted-foreground">
        Login disabled (set <code className="font-mono">VITE_GOOGLE_CLIENT_ID</code>)
      </span>
    )
  }
  return <div ref={ref} />
}
