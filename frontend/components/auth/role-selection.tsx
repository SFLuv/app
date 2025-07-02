"use client"

import { Button } from "@/components/ui/button"
import { ArrowLeft, ArrowRight } from "lucide-react"

interface RoleSelectionProps {
  onSelectRole: (role: "user" | "merchant" | "admin") => void
  onBack: () => void
}

export function RoleSelection({ onSelectRole, onBack }: RoleSelectionProps) {
  return (
    <div className="space-y-6">
      <div className="space-y-2 text-center">
        <h1 className="text-3xl font-bold">Choose your role</h1>
        <p className="text-gray-500 dark:text-gray-400">Select how you want to use SFLuv</p>
      </div>

      <div className="grid gap-4">
        <Button
          variant="outline"
          className="flex items-center justify-between h-auto p-4 text-left"
          onClick={() => onSelectRole("user")}
        >
          <div>
            <div className="font-medium">Community Member</div>
            <div className="text-sm text-gray-500 dark:text-gray-400">
              Volunteer, earn SFLuv, and spend at local businesses
            </div>
          </div>
          <ArrowRight className="h-5 w-5" />
        </Button>

        <Button
          variant="outline"
          className="flex items-center justify-between h-auto p-4 text-left"
          onClick={() => onSelectRole("merchant")}
        >
          <div>
            <div className="font-medium">Local Business</div>
            <div className="text-sm text-gray-500 dark:text-gray-400">
              Accept SFLuv as payment and support the community
            </div>
          </div>
          <ArrowRight className="h-5 w-5" />
        </Button>

        <Button
          variant="outline"
          className="flex items-center justify-between h-auto p-4 text-left"
          onClick={() => onSelectRole("admin")}
        >
          <div>
            <div className="font-medium">Administrator</div>
            <div className="text-sm text-gray-500 dark:text-gray-400">Manage the SFLuv platform and user roles</div>
          </div>
          <ArrowRight className="h-5 w-5" />
        </Button>
      </div>

      <Button variant="ghost" onClick={onBack} className="w-full">
        <ArrowLeft className="mr-2 h-4 w-4" /> Back
      </Button>
    </div>
  )
}
