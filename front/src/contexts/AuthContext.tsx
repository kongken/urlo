import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react"
import { api, type AuthUser } from "@/lib/api"

interface AuthState {
  user: AuthUser | null
  loading: boolean
  loginWithGoogle: (idToken: string) => Promise<AuthUser>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      const u = await api.me()
      setUser(u)
    } catch {
      setUser(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const loginWithGoogle = useCallback(async (idToken: string) => {
    const u = await api.loginWithGoogle(idToken)
    setUser(u)
    return u
  }, [])

  const logout = useCallback(async () => {
    await api.logout()
    setUser(null)
  }, [])

  return (
    <AuthContext.Provider value={{ user, loading, loginWithGoogle, logout, refresh }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider")
  return ctx
}
