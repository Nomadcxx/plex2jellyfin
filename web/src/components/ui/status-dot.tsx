import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const statusDotVariants = cva(
  "inline-block rounded-full animate-in fade-in zoom-in duration-300",
  {
    variants: {
      status: {
        online: "bg-emerald-500",
        offline: "bg-zinc-500",
        warning: "bg-amber-500",
        error: "bg-red-500",
        active: "bg-violet-500",
        blue: "bg-blue-500",
      },
      size: {
        sm: "h-1.5 w-1.5",
        default: "h-2 w-2",
        lg: "h-3 w-3",
      },
      pulse: {
        true: "animate-pulse",
        false: "",
      },
    },
    defaultVariants: {
      status: "online",
      size: "default",
      pulse: false,
    },
  }
)

export interface StatusDotProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof statusDotVariants> {}

function StatusDot({ className, status, size, pulse, ...props }: StatusDotProps) {
  return (
    <span className="relative flex items-center justify-center">
      {pulse && (
        <span
          className={cn(
            "absolute inline-flex h-full w-full animate-ping rounded-full opacity-75",
            statusDotVariants({ status, size: undefined, pulse: false }).replace(
              /h-\d+(?:\.\d+)? w-\d+(?:\.\d+)?/,
              ""
            )
          )}
        />
      )}
      <span
        className={cn(statusDotVariants({ status, size, pulse: false }), className)}
        {...props}
      />
    </span>
  )
}

export { StatusDot }
