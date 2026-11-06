"use client";

import { Input } from "@/shared/components";

/** Reusable endpoint row component */
export default function EndpointRow({ label, url, copyId, copied, onCopy, badge, actions }) {
  const isCopied = copied === copyId;

  return (
    <div className="flex items-center gap-2 group">
      <span
        className={`text-xs font-mono font-medium px-2.5 py-1.5 rounded-xl shrink-0 min-w-[88px] text-center border transition-colors ${
          badge === "CF" || badge === "TS"
            ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 border-brand-500/20"
            : "bg-surface-2 text-text-muted border-border/70"
        }`}
      >
        {label}
      </span>
      <div className="flex-1 min-w-0">
        <Input
          value={url}
          readOnly
          className="font-mono text-xs sm:text-sm"
          inputClassName="font-mono text-xs sm:text-sm bg-surface-2/90 select-all"
        />
      </div>
      <button
        onClick={() => onCopy(url, copyId)}
        title={isCopied ? "Copied!" : "Copy to clipboard"}
        aria-label={isCopied ? "Copied" : `Copy ${label} URL`}
        className={`size-10 rounded-xl flex items-center justify-center border transition-all cursor-pointer shrink-0 active:scale-95 ${
          isCopied
            ? "bg-green-500/15 border-green-500/30 text-green-600 dark:text-green-400"
            : "bg-surface-2 border-border/70 text-text-muted hover:text-brand-500 hover:border-brand-500/30 hover:bg-brand-500/5"
        }`}
      >
        <span className="material-symbols-outlined text-[18px]">
          {isCopied ? "check" : "content_copy"}
        </span>
      </button>
      {actions}
    </div>
  );
}
