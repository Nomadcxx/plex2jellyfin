import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const cardVariants = cva(
  "rounded-md border bg-card text-card-foreground shadow-sm relative overflow-hidden",
  {
    variants: {
      variant: {
        default: "border-border",
        interactive:
          "border-border hover:border-border/80 hover:bg-accent/5 transition-all duration-200 cursor-pointer",
        gradient:
          "group border-border/50 bg-background/50 backdrop-blur-sm hover:bg-accent/5 hover:border-border transition-all duration-300",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

export interface CardProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof cardVariants> {
  gradientColor?: "violet" | "blue" | "emerald" | "amber" | "rose"
}

const gradientMap = {
  violet: "from-violet-500/10",
  blue: "from-blue-500/10",
  emerald: "from-emerald-500/10",
  amber: "from-amber-500/10",
  rose: "from-rose-500/10",
}

const Card = React.forwardRef<HTMLDivElement, CardProps>(
  ({ className, variant, gradientColor, children, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(cardVariants({ variant }), className)}
      {...props}
    >
      {variant === "gradient" && gradientColor && (
        <div
          className={cn(
            "absolute inset-0 bg-gradient-to-br to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300 pointer-events-none",
            gradientMap[gradientColor]
          )}
        />
      )}
      <div className="relative z-10 h-full">{children}</div>
    </div>
  )
)
Card.displayName = "Card"

const CardHeader = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn("flex flex-col space-y-1.5 p-6", className)}
      {...props}
    />
  )
)
CardHeader.displayName = "CardHeader"

const CardTitle = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLHeadingElement>>(
  ({ className, ...props }, ref) => (
    <h3
      ref={ref}
      className={cn("text-lg font-semibold leading-none tracking-tight", className)}
      {...props}
    />
  )
)
CardTitle.displayName = "CardTitle"

const CardDescription = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLParagraphElement>>(
  ({ className, ...props }, ref) => (
    <p
      ref={ref}
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
  )
)
CardDescription.displayName = "CardDescription"

const CardContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("p-6 pt-0", className)} {...props} />
  )
)
CardContent.displayName = "CardContent"

const CardFooter = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn("flex items-center p-6 pt-0", className)}
      {...props}
    />
  )
)
CardFooter.displayName = "CardFooter"

export { Card, CardHeader, CardFooter, CardTitle, CardDescription, CardContent }
