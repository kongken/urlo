import { BrowserRouter, Routes, Route } from "react-router-dom"
import { Layout } from "@/components/Layout"
import { Toaster } from "@/components/ui/sonner"
import Landing from "@/pages/Landing"
import Dashboard from "@/pages/Dashboard"
import Analytics from "@/pages/Analytics"
import Settings from "@/pages/Settings"

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Landing />} />
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="analytics" element={<Analytics />} />
          <Route path="analytics/:code" element={<Analytics />} />
          <Route path="settings" element={<Settings />} />
        </Route>
      </Routes>
      <Toaster richColors />
    </BrowserRouter>
  )
}
