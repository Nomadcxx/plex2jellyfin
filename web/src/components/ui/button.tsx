import * as React from "react"
import { cn } from "@/lib/utils"

const buttonVariants = ({
  variant = "default",
  size = "default",
  className = "",
}: {
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link"
  size?: "default" | "sm" | "lg" | "icon"
  className?: string
}) => {
  const variants = {
    default: "bg-zinc-100 text-zinc-900 hover:bg-zinc-200",
    destructive: "bg-red-500 text-white hover:bg-red-600",
    outline: "border border-zinc-700 bg-transparent hover:bg-zinc-800",
    secondary: "bg-zinc-800 text-zinc-100 hover:bg-zinc-700",
    ghost: "hover:bg-zinc-800 hover:text-zinc-100",
    link: "text-zinc-100 underline-offset-4 hover:underline",
  }
  
  const sizes = {
    default: "h-10 px-4 py-2",
    sm: "h-9 rounded-md px-3",
    lg: "h-11 rounded-md px-8",
    icon: "h-10 w-10",
  }
  
  return `inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 ${variants[variant]} ${sizes[size]} ${className}`
}

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link"
  size?: "default" | "sm" | "lg" | "icon"
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return (
      <button
        className={buttonVariants({ variant, size, className })}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = "Button"

export { Button, buttonVariants }
