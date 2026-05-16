import { createContext } from "react"

import { type AuthUser } from "@/lib/api"

export interface AuthState {
  user: AuthUser | null
  loading: boolean
  loginWithGoogle: (idToken: string) => Promise<AuthUser>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

export const AuthContext = createContext<AuthState | null>(null)
