"use client";

import PropTypes from "prop-types";
import { cn } from "@/shared/utils/cn";

const ICON_SIZES = {
  sm: "size-7",
  md: "size-9",
  lg: "size-11",
  xl: "size-14",
};

const TITLE_SIZES = {
  sm: "text-sm",
  md: "text-base",
  lg: "text-xl",
  xl: "text-2xl",
};

const SUBTITLE_SIZES = {
  sm: "text-[9px] tracking-[0.2em]",
  md: "text-[10px] tracking-[0.22em]",
  lg: "text-xs tracking-[0.25em]",
  xl: "text-sm tracking-[0.28em]",
};

// Reusable vector router mark
function IconMark({ size = "md", customClass }) {
  return (
    <div
      className={cn(
        "relative flex items-center justify-center shrink-0 rounded-xl bg-gradient-to-br from-brand-400 via-brand-500 to-brand-700 shadow-[var(--shadow-warm)] ring-1 ring-white/20 select-none overflow-hidden",
        customClass || ICON_SIZES[size]
      )}
    >
      <svg viewBox="0 0 48 48" fill="none" className="size-full p-2" xmlns="http://www.w3.org/2000/svg">
        {/* Central Core Glowing Node */}
        <circle cx="24" cy="24" r="4.5" fill="#FFFFFF" />
        <circle cx="24" cy="24" r="2.2" fill="#C2410C" />

        {/* Top-Left Inflow Stream */}
        <path d="M 13 14 L 18 19 L 21 21" stroke="white" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
        <circle cx="13" cy="14" r="2.2" fill="white" />

        {/* Top-Right Inflow Stream */}
        <path d="M 35 14 L 30 19 L 27 21" stroke="white" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
        <circle cx="35" cy="14" r="2.2" fill="white" />

        {/* Bottom Multi-Provider Outflow */}
        <path d="M 24 28.5 L 24 35" stroke="white" strokeWidth="2.4" strokeLinecap="round" />
        <path d="M 24 35 L 17 38" stroke="white" strokeWidth="2" strokeLinecap="round" opacity="0.9" />
        <path d="M 24 35 L 31 38" stroke="white" strokeWidth="2" strokeLinecap="round" opacity="0.9" />
        <circle cx="17" cy="38" r="1.8" fill="white" />
        <circle cx="31" cy="38" r="1.8" fill="white" />

        {/* Data Packets */}
        <circle cx="28.5" cy="17.5" r="1.2" fill="#FEF08A" />
        <circle cx="19.5" cy="17.5" r="1.2" fill="#FEF08A" />
      </svg>
    </div>
  );
}

IconMark.propTypes = {
  size: PropTypes.oneOf(["sm", "md", "lg", "xl"]),
  customClass: PropTypes.string,
};

export default function Logo({
  variant = "full",
  size = "md",
  showSubtitle = true,
  className,
}) {
  if (variant === "mark") {
    return <IconMark size={size} customClass={className} />;
  }

  if (variant === "stacked") {
    return (
      <div className={cn("flex flex-col items-center text-center gap-3 select-none", className)}>
        <IconMark
          size={size}
          customClass={size === "xl" ? "size-16 rounded-2xl shadow-xl" : undefined}
        />
        <div className="flex flex-col items-center">
          <div className={cn("font-bold tracking-tight text-text-main leading-tight", TITLE_SIZES[size])}>
            <span>lampt</span>
            <span className="text-brand-500">.works</span>
          </div>
          {showSubtitle && (
            <div className="mt-1 inline-flex items-center px-2 py-0.5 rounded-md bg-brand-500/10 border border-brand-500/20">
              <span className={cn("font-mono font-bold uppercase text-brand-600 dark:text-brand-400", SUBTITLE_SIZES[size])}>
                router
              </span>
            </div>
          )}
        </div>
      </div>
    );
  }

  // Default: "full" horizontal lockup
  return (
    <div className={cn("flex items-center gap-3 select-none min-w-0", className)}>
      <IconMark size={size} />
      <div className="flex flex-col min-w-0">
        <div className={cn("font-bold tracking-tight text-text-main leading-none truncate", TITLE_SIZES[size])}>
          <span>lampt</span>
          <span className="text-brand-500">.works</span>
        </div>
        {showSubtitle && (
          <div className="flex items-center gap-1.5 mt-1">
            <span className={cn("font-mono font-bold uppercase text-brand-600 dark:text-brand-400 leading-none", SUBTITLE_SIZES[size])}>
              router
            </span>
          </div>
        )}
      </div>
    </div>
  );
}

Logo.propTypes = {
  variant: PropTypes.oneOf(["full", "stacked", "mark", "text"]),
  size: PropTypes.oneOf(["sm", "md", "lg", "xl"]),
  showSubtitle: PropTypes.bool,
  className: PropTypes.string,
};
