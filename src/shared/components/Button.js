"use client";

import { cn } from "@/shared/utils/cn";

const variants = {
  primary:
    "bg-gradient-to-r from-brand-500 to-brand-600 hover:from-brand-600 hover:to-brand-700 text-white shadow-sm hover:shadow-[0_4px_16px_0_rgba(229,106,74,0.3)] disabled:from-surface-3 disabled:to-surface-3 disabled:text-text-muted disabled:shadow-none",
  secondary:
    "bg-surface-2 hover:bg-surface-3 text-text-main border border-border/80 shadow-sm disabled:opacity-50",
  outline:
    "border border-border/80 text-text-main hover:bg-surface-2 hover:border-brand-500/40 disabled:opacity-50",
  ghost:
    "text-text-muted hover:bg-surface-2 hover:text-text-main disabled:opacity-50",
  danger:
    "bg-gradient-to-r from-red-500 to-red-600 hover:from-red-600 hover:to-red-700 text-white shadow-sm hover:shadow-[0_4px_14px_0_rgba(239,68,68,0.3)] disabled:opacity-50",
  success:
    "bg-gradient-to-r from-green-500 to-green-600 hover:from-green-600 hover:to-green-700 text-white shadow-sm hover:shadow-[0_4px_14px_0_rgba(34,197,94,0.3)] disabled:opacity-50",
};

const sizes = {
  xs: "h-6 px-2.5 text-[11px] rounded-lg gap-1",
  sm: "h-8 px-3 text-xs rounded-lg gap-1.5",
  md: "h-9 sm:h-10 px-4 text-sm rounded-xl gap-2",
  lg: "h-11 sm:h-12 px-5 text-sm sm:text-base rounded-xl gap-2.5",
};

export default function Button({
  children,
  variant = "primary",
  size = "md",
  icon,
  iconRight,
  disabled = false,
  loading = false,
  fullWidth = false,
  className,
  ...props
}) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center font-medium transition-all duration-150 ease-out cursor-pointer select-none",
        "focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:outline-none",
        "active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed disabled:active:scale-100",
        variants[variant],
        sizes[size],
        fullWidth && "w-full",
        className
      )}
      disabled={disabled || loading}
      aria-busy={loading}
      {...props}
    >
      {loading ? (
        <span className={cn("material-symbols-outlined animate-spin", size === "xs" ? "text-[14px]" : "text-[18px]")}>progress_activity</span>
      ) : icon ? (
        <span className={cn("material-symbols-outlined", size === "xs" ? "text-[14px]" : "text-[18px]")}>{icon}</span>
      ) : null}
      {children}
      {iconRight && !loading && (
        <span className={cn("material-symbols-outlined", size === "xs" ? "text-[14px]" : "text-[18px]")}>{iconRight}</span>
      )}
    </button>
  );
}
