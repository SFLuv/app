import type * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const badgeVariants = cva(
  "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
  {
    variants: {
      variant: {
        default: "border-primary/30 bg-primary/12 text-primary hover:bg-primary/18",
        secondary: "border-border/60 bg-secondary/80 text-secondary-foreground hover:bg-secondary",
        destructive: "border-destructive/30 bg-destructive/12 text-destructive hover:bg-destructive/18",
        outline: "border-border bg-card text-foreground",
        success: "border-green-600/30 bg-green-500/15 text-green-700 dark:text-green-300 hover:bg-green-500/20",
        warning: "border-amber-600/30 bg-amber-500/15 text-amber-700 dark:text-amber-300 hover:bg-amber-500/20",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
)

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />
}

export { Badge, badgeVariants }
