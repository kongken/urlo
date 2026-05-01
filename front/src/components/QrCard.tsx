import { useRef } from "react"
import { QRCodeCanvas } from "qrcode.react"
import { Download } from "lucide-react"
import { Button } from "@/components/ui/button"

interface QrCardProps {
  value: string
  filename?: string
  size?: number
}

export function QrCard({ value, filename = "qrcode", size = 160 }: QrCardProps) {
  const wrapperRef = useRef<HTMLDivElement>(null)

  function download() {
    const canvas = wrapperRef.current?.querySelector("canvas")
    if (!canvas) return
    const url = canvas.toDataURL("image/png")
    const a = document.createElement("a")
    a.href = url
    a.download = `${filename}.png`
    document.body.appendChild(a)
    a.click()
    a.remove()
  }

  return (
    <div className="flex items-center gap-4">
      <div
        ref={wrapperRef}
        className="rounded-lg bg-white p-2 border shadow-sm shrink-0"
      >
        <QRCodeCanvas
          value={value}
          size={size}
          marginSize={1}
          level="M"
        />
      </div>
      <Button variant="outline" size="sm" onClick={download}>
        <Download className="h-4 w-4 mr-2" /> Download PNG
      </Button>
    </div>
  )
}
