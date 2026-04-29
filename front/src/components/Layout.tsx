import { Link, NavLink, Outlet } from "react-router-dom"
import { Link2 } from "lucide-react"
import { cn } from "@/lib/utils"

const navItems = [
  { to: "/", label: "Home", end: true },
  { to: "/dashboard", label: "My Links" },
  { to: "/analytics", label: "Analytics" },
  { to: "/settings", label: "Settings" },
]

export function Layout() {
  return (
    <div className="min-h-svh bg-background text-foreground">
      <header className="border-b bg-card/50 backdrop-blur sticky top-0 z-10">
        <div className="mx-auto flex max-w-6xl items-center gap-6 px-6 py-4">
          <Link to="/" className="flex items-center gap-2 font-bold text-lg">
            <span className="grid h-8 w-8 place-items-center rounded-lg bg-primary text-primary-foreground">
              <Link2 className="h-4 w-4" />
            </span>
            <span>urlo</span>
          </Link>
          <nav className="flex items-center gap-1 text-sm">
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
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-6 py-10">
        <Outlet />
      </main>
    </div>
  )
}
