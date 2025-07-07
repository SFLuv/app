import type React from "react"
import Image from "next/image"
import { ThemeProvider } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

interface AuthLayoutProps {
  children: React.ReactNode
  title: string
  description?: string
  className?: string
}

export function AuthLayout({ children, title, description, className }: AuthLayoutProps) {
  return (
    <ThemeProvider>
      <div className="min-h-screen flex flex-col items-center justify-center bg-[#d3d3d3] dark:bg-[#1a1a1a] p-4">
        <div className="w-full max-w-md">
          <div className="flex justify-center mb-8">
            <div className="relative overflow-hidden bg-white dark:bg-[#2a2a2a] rounded-2xl p-6 shadow-lg">
              {/* Wave background */}
              <div className="absolute inset-0 z-0 opacity-10">
                <svg
                  className="absolute bottom-0 left-0 w-full h-full transform"
                  viewBox="0 0 1440 320"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <path
                    d="M0,192L48,197.3C96,203,192,213,288,229.3C384,245,480,267,576,250.7C672,235,768,181,864,181.3C960,181,1056,235,1152,234.7C1248,235,1344,181,1392,154.7L1440,128L1440,320L1392,320C1344,320,1248,320,1152,320C1056,320,960,320,864,320C768,320,672,320,576,320C480,320,384,320,288,320C192,320,96,320,48,320L0,320Z"
                    fill="#eb6c6c"
                  />
                </svg>
              </div>

              {/* Logo */}
              <div className="relative z-10">
                <Image
                  src="https://hebbkx1anhila5yf.public.blob.vercel-storage.com/SFLUV%20Currency%20Symbol%20Logo-vy4PMmBMXIecSbbo0Ozx2nQVRmUyru.png"
                  alt="SFLuv"
                  width={150}
                  height={60}
                  priority
                  className="h-12 w-auto"
                />
              </div>
            </div>
          </div>

          <div className={cn("bg-white dark:bg-[#2a2a2a] rounded-lg shadow-lg p-6", className)}>
            <div className="text-center mb-6">
              <h1 className="text-2xl font-bold text-black dark:text-white">{title}</h1>
              {description && <p className="text-gray-800 dark:text-gray-200 mt-2">{description}</p>}
            </div>
            {children}
          </div>
        </div>
      </div>
    </ThemeProvider>
  )
}
