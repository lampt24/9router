"use client";

import Link from "next/link";
import { Card } from "@/shared/components";
import Image from "next/image";

/**
 * Clickable card for MITM tools — navigates to /dashboard/mitm on click.
 */
export default function MitmLinkCard({ tool }) {
  return (
    <Link href="/dashboard/mitm" className="group block h-full">
      <Card
        padding="sm"
        className="h-full overflow-hidden hover:border-purple-500/40 hover:shadow-[0_4px_20px_-2px_rgba(168,85,247,0.15)] hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
      >
        <div className="flex h-full items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="size-10 rounded-xl bg-surface-2 border border-border/60 flex items-center justify-center shrink-0 group-hover:scale-105 transition-transform">
              <Image
                src={tool.image}
                alt={tool.name}
                width={28}
                height={28}
                className="size-7 object-contain rounded-md"
                sizes="28px"
                onError={(e) => {
                  e.target.style.display = "none";
                }}
                loading="lazy"
                decoding="async"
              />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h3 className="font-semibold text-sm text-text-main group-hover:text-purple-500 transition-colors truncate">
                  {tool.name}
                </h3>
                <span className="px-2 py-0.5 text-[10px] font-medium bg-purple-500/10 text-purple-700 dark:text-purple-300 border border-purple-500/20 rounded-full">
                  MITM
                </span>
              </div>
              <p className="text-xs text-text-muted truncate mt-0.5">{tool.description}</p>
            </div>
          </div>
          <span className="material-symbols-outlined text-text-muted text-[20px] shrink-0 group-hover:translate-x-0.5 group-hover:text-purple-500 transition-all">
            chevron_right
          </span>
        </div>
      </Card>
    </Link>
  );
}
