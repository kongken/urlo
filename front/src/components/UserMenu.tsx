import { LogOut } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/contexts/AuthContext"
import { LoginButton } from "@/components/LoginButton"

export function UserMenu() {
  const { user, loading, logout } = useAuth()

  if (loading) return null
  if (!user) return <LoginButton />

  const display = user.name || user.email || user.sub
  return (
    <div className="flex items-center gap-3">
      <span className="text-sm text-muted-foreground" title={user.email}>
        {display}
      </span>
      <Button
        variant="ghost"
        size="sm"
        onClick={async () => {
          try {
            await logout()
            toast.success("Signed out")
          } catch (e) {
            toast.error(e instanceof Error ? e.message : "logout failed")
          }
        }}
      >
        <LogOut className="h-4 w-4 mr-1" />
        Logout
      </Button>
    </div>
  )
}
