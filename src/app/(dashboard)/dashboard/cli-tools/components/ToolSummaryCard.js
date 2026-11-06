"use client";

import Link from "next/link";
import Image from "next/image";
import { Card } from "@/shared/components";

// Derive simple connected/configured/not-installed status from API payload
function getStatus(status, tool) {
  if (tool?.configType === "guide")
    return { label: "Guide", cls: "bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20", dotCls: "bg-blue-500" };
  if (!status)
    return { label: "Unknown", cls: "bg-gray-500/10 text-gray-500 border-gray-500/20", dotCls: "bg-gray-400" };
  if (!status.installed)
    return { label: "Not installed", cls: "bg-gray-500/10 text-gray-500 border-gray-500/20", dotCls: "bg-gray-400" };
  if (status.has9Router)
    return { label: "Connected", cls: "bg-green-500/10 text-green-700 dark:text-green-300 border-green-500/20", dotCls: "bg-green-500", pulse: true };
  return { label: "Not configured", cls: "bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/20", dotCls: "bg-amber-500" };
}

export default function ToolSummaryCard({ toolId, tool, status }) {
  const s = getStatus(status, tool);
  return (
    <Link href={`/dashboard/cli-tools/${toolId}`} className="group block h-full">
      <Card
        padding="sm"
        className="h-full overflow-hidden hover:border-brand-500/40 hover:shadow-[var(--shadow-warm)] hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
      >
        <div className="flex h-full items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="size-10 rounded-xl bg-surface-2 border border-border/60 flex items-center justify-center shrink-0 group-hover:scale-105 transition-transform">
              {tool.image ? (
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
              ) : tool.icon ? (
                <span className="material-symbols-outlined text-[24px]" style={{ color: tool.color }}>
                  {tool.icon}
                </span>
              ) : null}
            </div>
            <div className="min-w-0 flex-1">
              <h3 className="font-semibold text-sm text-text-main truncate group-hover:text-brand-500 transition-colors">
                {tool.name}
              </h3>
              <div className="flex items-center gap-1.5 mt-1">
                <span className={`inline-flex items-center gap-1 px-2 py-0.5 text-[10px] font-medium rounded-full border ${s.cls}`}>
                  <span className="relative flex size-1.5">
                    {s.pulse && (
                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75" />
                    )}
                    <span className={`relative inline-flex rounded-full size-1.5 ${s.dotCls}`} />
                  </span>
                  {s.label}
                </span>
              </div>
            </div>
          </div>
          <span className="material-symbols-outlined text-text-muted text-[20px] shrink-0 group-hover:translate-x-0.5 group-hover:text-brand-500 transition-all">
            chevron_right
          </span>
        </div>
      </Card>
    </Link>
  );
}
