import { useState } from "react"
import { Link, NavLink, Outlet, useLocation } from "react-router-dom"
import { Link2, Menu, X } from "lucide-react"
import { cn } from "@/lib/utils"
import { UserMenu } from "@/components/UserMenu"

const navItems = [
  { to: "/", label: "Home", end: true },
  { to: "/expand", label: "URL Restore" },
  { to: "/dashboard", label: "My Links" },
  { to: "/analytics", label: "Analytics" },
  { to: "/settings", label: "Settings" },
  { to: "/api-docs", label: "API Docs" },
]

export function Layout() {
  const [mobileOpen, setMobileOpen] = useState(false)
  const location = useLocation()

  const closeMenu = () => setMobileOpen(false)

  return (
    <div className="min-h-svh bg-background text-foreground">
      <header className="border-b bg-card/50 backdrop-blur sticky top-0 z-10">
        <div className="mx-auto flex max-w-6xl items-center gap-4 px-4 py-3 md:gap-6 md:px-6 md:py-4">
          <Link to="/" className="flex items-center gap-2 font-bold text-lg" onClick={closeMenu}>
            <span className="grid h-8 w-8 place-items-center rounded-lg bg-primary text-primary-foreground">
              <Link2 className="h-4 w-4" />
            </span>
            <span>urlo</span>
          </Link>
          <nav className="hidden md:flex items-center gap-1 text-sm">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  cn(
                    "rounded-md px-3 py-1.5 transition-colors",
                    isActive
                      ? "bg-secondary text-secondary-foreground font-medium"
                      : "text-muted-foreground hover:text-foreground",
                  )
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto hidden md:block">
            <UserMenu />
          </div>
          <button
            type="button"
            className="ml-auto md:hidden p-2 rounded-md hover:bg-secondary transition-colors"
            onClick={() => setMobileOpen(!mobileOpen)}
            aria-label="Toggle menu"
          >
            {mobileOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
          </button>
        </div>

        {mobileOpen && (
          <div className="md:hidden border-t px-4 py-3 space-y-2 bg-card/95 backdrop-blur">
            <nav className="flex flex-col gap-1 text-sm">
              {navItems.map((item) => {
                const isActive = item.end
                  ? location.pathname === item.to
                  : location.pathname.startsWith(item.to)
                return (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.end}
                    onClick={closeMenu}
                    className={cn(
                      "rounded-md px-3 py-2 transition-colors",
                      isActive
                        ? "bg-secondary text-secondary-foreground font-medium"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {item.label}
                  </NavLink>
                )
              })}
            </nav>
            <div className="pt-2 border-t">
              <UserMenu />
            </div>
          </div>
        )}
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6 md:px-6 md:py-10">
        <Outlet />
      </main>
    </div>
  )
}
