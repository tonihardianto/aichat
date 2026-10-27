import * as React from "react";
import { cn } from "@/lib/utils";

type BubbleVariant = "default" | "secondary" | "muted" | "tinted" | "outline" | "ghost" | "destructive";

const bubbleVariants: Record<BubbleVariant, string> = {
  default: "group/bubble",
  secondary: "group/bubble",
  muted: "group/bubble",
  tinted: "group/bubble",
  outline: "group/bubble",
  ghost: "group/bubble",
  destructive: "group/bubble",
};

export function BubbleGroup({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="bubble-group" className={cn("flex min-w-0 flex-col gap-2", className)} {...props} />;
}

export function Bubble({ align = "start", variant = "default", className, ...props }: React.ComponentProps<"div"> & { align?: "start" | "end"; variant?: BubbleVariant }) {
  return (
    <div
      data-slot="bubble"
      data-variant={variant}
      data-align={align}
      className={cn(
        "group/bubble relative flex w-fit max-w-[80%] min-w-0 flex-col gap-1",
        align === "end" && "ml-auto",
        bubbleVariants[variant],
        className
      )}
      {...props}
    />
  );
}

export function BubbleContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="bubble-content"
      className={cn(
        "w-fit max-w-full min-w-0 overflow-hidden rounded-2xl border px-4 py-2.5 text-sm leading-relaxed break-words",
        "group-data-[variant=default]/bubble:border-border/60 group-data-[variant=default]/bubble:bg-secondary group-data-[variant=default]/bubble:text-secondary-foreground group-data-[variant=default]/bubble:shadow-xs",
        "group-data-[variant=secondary]/bubble:border-border/60 group-data-[variant=secondary]/bubble:bg-secondary group-data-[variant=secondary]/bubble:text-secondary-foreground group-data-[variant=secondary]/bubble:shadow-xs",
        "group-data-[variant=muted]/bubble:border-border/40 group-data-[variant=muted]/bubble:bg-muted group-data-[variant=muted]/bubble:text-foreground",
        "group-data-[variant=tinted]/bubble:border-primary/20 group-data-[variant=tinted]/bubble:bg-primary/10 group-data-[variant=tinted]/bubble:text-foreground dark:group-data-[variant=tinted]/bubble:bg-primary/20",
        "group-data-[variant=outline]/bubble:border-border group-data-[variant=outline]/bubble:bg-background",
        "group-data-[variant=ghost]/bubble:border-none group-data-[variant=ghost]/bubble:rounded-none group-data-[variant=ghost]/bubble:bg-transparent group-data-[variant=ghost]/bubble:p-0",
        "group-data-[variant=destructive]/bubble:border-destructive/20 group-data-[variant=destructive]/bubble:bg-destructive/10 group-data-[variant=destructive]/bubble:text-destructive dark:group-data-[variant=destructive]/bubble:bg-destructive/20",
        className
      )}
      {...props}
    />
  );
}

export function BubbleReactions({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="bubble-reactions" className={cn("absolute z-10 flex w-fit shrink-0 items-center justify-center gap-1 rounded-full bg-muted px-1.5 py-0.5 text-sm ring-3 ring-card has-[button]:p-0", className)} {...props} />;
}
