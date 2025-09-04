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
          <div className="flex justify-center mb-8 relative">
            <div className="absolute inset-0 bg-[#eb6c6c] opacity-10 rounded-xl blur-xl transform -translate-y-4"></div>
            <div className="bg-white dark:bg-[#2a2a2a] rounded-xl py-2 px-8 shadow-lg relative z-10 border border-gray-100 dark:border-gray-800 w-[220px]">
              <Image
                src="https://hebbkx1anhila5yf.public.blob.vercel-storage.com/SFLUV%20Currency%20Symbol%20Logo-vy4PMmBMXIecSbbo0Ozx2nQVRmUyru.png"
                alt="SFLuv"
                width={150}
                height={60}
                priority
                className="w-[150px] h-auto mx-auto"
              />
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
