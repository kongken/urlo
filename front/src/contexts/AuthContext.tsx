import { useCallback, useEffect, useState, type ReactNode } from "react"
import { api, type AuthUser } from "@/lib/api"
import { AuthContext } from "@/contexts/auth-context"

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
