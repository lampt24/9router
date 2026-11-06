"use client";

import PropTypes from "prop-types";
import Card from "@/shared/components/Card";

const fmt = (n) => new Intl.NumberFormat().format(n || 0);
const fmtCost = (n) => `$${(n || 0).toFixed(2)}`;

export default function OverviewCards({ stats }) {
  return (
    <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-5 sm:gap-4">
      <Card hover className="flex min-w-0 flex-col items-center text-center gap-1 p-4 rounded-2xl">
        <span className="text-text-muted text-xs uppercase font-semibold tracking-wider">Total Requests</span>
        <span className="w-full truncate text-2xl font-bold tracking-tight text-text-main tabular-nums mt-1" title={fmt(stats.totalRequests)}>
          {fmt(stats.totalRequests)}
        </span>
      </Card>
      <Card hover className="flex min-w-0 flex-col items-center text-center gap-1 p-4 rounded-2xl">
        <span className="text-text-muted text-xs uppercase font-semibold tracking-wider">Input Tokens</span>
        <span className="w-full truncate text-2xl font-bold tracking-tight text-brand-600 dark:text-brand-400 tabular-nums mt-1" title={fmt(stats.totalPromptTokens)}>
          {fmt(stats.totalPromptTokens)}
        </span>
      </Card>
      <Card hover className="flex min-w-0 flex-col items-center text-center gap-1 p-4 rounded-2xl">
        <span className="text-text-muted text-xs uppercase font-semibold tracking-wider">Cached Tokens</span>
        <span className="w-full truncate text-2xl font-bold tracking-tight text-blue-600 dark:text-blue-400 tabular-nums mt-1" title={fmt(stats.totalCachedTokens)}>
          {fmt(stats.totalCachedTokens)}
        </span>
      </Card>
      <Card hover className="flex min-w-0 flex-col items-center text-center gap-1 p-4 rounded-2xl">
        <span className="text-text-muted text-xs uppercase font-semibold tracking-wider">Output Tokens</span>
        <span className="w-full truncate text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400 tabular-nums mt-1" title={fmt(stats.totalCompletionTokens)}>
          {fmt(stats.totalCompletionTokens)}
        </span>
      </Card>
      <Card hover className="flex min-w-0 flex-col items-center text-center gap-1 p-4 rounded-2xl">
        <span className="text-text-muted text-xs uppercase font-semibold tracking-wider">Est. Cost</span>
        <span className="w-full truncate text-2xl font-bold tracking-tight text-amber-600 dark:text-amber-400 tabular-nums mt-1" title={`~${fmtCost(stats.totalCost)}`}>
          ~{fmtCost(stats.totalCost)}
        </span>
        <span className="text-[10px] text-text-muted mt-0.5">Estimated, not actual billing</span>
      </Card>
    </div>
  );
}

OverviewCards.propTypes = {
  stats: PropTypes.object.isRequired,
};
