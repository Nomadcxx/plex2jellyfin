import * as React from "react"
import { cn } from "@/lib/utils"

const alertVariants = ({
  variant = "default",
  className = "",
}: {
  variant?: "default" | "destructive"
  className?: string
}) => {
  const variants = {
    default: "bg-zinc-900 border-zinc-700 text-zinc-100",
    destructive: "bg-terminal-red/10 border-terminal-red/30 text-terminal-red",
  }
  
  return `relative w-full rounded-lg border p-4 ${variants[variant]} ${className}`
}

export interface AlertProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: "default" | "destructive"
}

const Alert = React.forwardRef<HTMLDivElement, AlertProps>(
  ({ className, variant, ...props }, ref) => (
    <div
      ref={ref}
      role="alert"
      className={alertVariants({ variant, className })}
      {...props}
    />
  )
)
Alert.displayName = "Alert"

const AlertDescription = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLParagraphElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn("text-sm", className)}
      {...props}
    />
  )
)
AlertDescription.displayName = "AlertDescription"

export { Alert, AlertDescription }
