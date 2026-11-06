"use client";

import { useState, useEffect } from "react";
import PropTypes from "prop-types";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/shared/utils/cn";
import { APP_CONFIG, UPDATER_CONFIG } from "@/shared/constants/config";
import { MEDIA_PROVIDER_KINDS } from "@/shared/constants/providers";
import { useCopyToClipboard } from "@/shared/hooks/useCopyToClipboard";
import useSettingsStore from "@/store/settingsStore";
import Button from "./Button";
import Logo from "./Logo";
import { ConfirmModal } from "./Modal";

// const VISIBLE_MEDIA_KINDS = ["embedding", "image", "imageToText", "tts", "stt", "webSearch", "webFetch", "video", "music"];
const VISIBLE_MEDIA_KINDS = ["embedding", "image", "video", "tts", "stt", "systemone"];
// Combined entry: webSearch + webFetch share one page at /dashboard/media-providers/web
const COMBINED_WEB_ITEM = { id: "web", label: "Web Fetch & Search", icon: "travel_explore", href: "/dashboard/media-providers/web" };

const navItems = [
  { href: "/dashboard/endpoint", label: "Endpoint & Key", icon: "api" },
  { href: "/dashboard/providers", label: "Providers", icon: "dns" },
  // { href: "/dashboard/basic-chat", label: "Basic Chat", icon: "chat" }, // Hidden
  { href: "/dashboard/combos", label: "Combo & Vision Adapter", icon: "layers" },
  { href: "/dashboard/usage", label: "Usage", icon: "bar_chart" },
  { href: "/dashboard/quota", label: "Quota Tracker", icon: "data_usage" },
  { href: "/dashboard/token-saver", label: "Token Saver", icon: "savings" },
  // { href: "/dashboard/pxpipe", label: "PXPIPE", icon: "image" },
  { href: "/dashboard/cli-tools", label: "CLI Tools", icon: "terminal" },
];

const debugItems = [
  { href: "/dashboard/console-log", label: "Console Log", icon: "terminal" },
  { href: "/dashboard/translator", label: "Translator", icon: "translate" },
];

const systemItems = [
  { href: "/dashboard/proxy-pools", label: "Proxy Pools", icon: "lan" },
  { href: "/dashboard/skills", label: "Skills", icon: "extension" },
];

export default function Sidebar({ onClose }) {
  const pathname = usePathname();
  const [mediaOpen, setMediaOpen] = useState(false);
  const [isDisconnected, setIsDisconnected] = useState(false);
  const [updateInfo, setUpdateInfo] = useState(null);
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);
  const [shutdownCountdown, setShutdownCountdown] = useState(0);
  const [enableTranslator, setEnableTranslator] = useState(false);
  const { copied, copy } = useCopyToClipboard(2000);

  const INSTALL_CMD = UPDATER_CONFIG.installCmdLatest;

  useEffect(() => {
    useSettingsStore.getState().fetchSettings().then((data) => {
      if (data?.enableTranslator) setEnableTranslator(true);
    });
  }, []);

  // Lazy check for new npm version in background after initial render
  useEffect(() => {
    const timer = setTimeout(() => {
      fetch("/api/version")
        .then(res => res.json())
        .then(data => { if (data.hasUpdate) setUpdateInfo(data); })
        .catch(() => {});
    }, 2500);
    return () => clearTimeout(timer);
  }, []);

  const isActive = (href) => {
    if (href === "/dashboard/endpoint") {
      return pathname === "/dashboard" || pathname.startsWith("/dashboard/endpoint");
    }
    return pathname.startsWith(href);
  };

  // Open manual update panel (no countdown yet — user must click Copy to trigger shutdown)
  const handleUpdate = () => {
    setShowUpdateModal(false);
    setIsUpdating(true);
  };

  // Triggered by Copy button inside ManualUpdatePanel: copy + countdown + shutdown
  const handleCopyAndShutdown = async () => {
    try { await navigator.clipboard.writeText(INSTALL_CMD); } catch { /* clipboard blocked */ }
    copy(INSTALL_CMD);
    let remaining = UPDATER_CONFIG.shutdownCountdownSec;
    setShutdownCountdown(remaining);
    const timer = setInterval(() => {
      remaining -= 1;
      setShutdownCountdown(remaining);
      if (remaining <= 0) {
        clearInterval(timer);
        fetch("/api/version/shutdown", { method: "POST" }).catch(() => {});
        setIsDisconnected(true);
      }
    }, 1000);
  };

  const handleCancelUpdate = () => {
    setIsUpdating(false);
    setShutdownCountdown(0);
  };

  // Note: legacy updater poll removed. New flow: copy install cmd + shutdown server,
  // user runs the command manually in another terminal.


  return (
    <>
      <aside className="flex w-72 flex-col border-r border-border/80 bg-vibrancy backdrop-blur-xl transition-colors duration-300 min-h-full select-none">
        {/* Traffic lights */}
        <div className="flex items-center gap-2 px-6 pt-5 pb-2">
          <div className="w-3 h-3 rounded-full bg-[#FF5F56] shadow-sm" />
          <div className="w-3 h-3 rounded-full bg-[#FFBD2E] shadow-sm" />
          <div className="w-3 h-3 rounded-full bg-[#27C93F] shadow-sm" />
        </div>

        {/* Logo */}
        <div className="px-5 py-3.5 flex flex-col gap-2">
          <Link href="/dashboard" className="flex items-center justify-between group">
            <Logo size="md" />
            <span className="px-1.5 py-0.5 text-[10px] font-mono font-medium rounded-full bg-surface-2 border border-border/60 text-text-muted shrink-0 group-hover:border-brand-500/30 transition-colors">
              v{APP_CONFIG.version}
            </span>
          </Link>
          {updateInfo && (
            <div className="flex flex-col gap-2 rounded-xl p-2.5 border border-green-500/30 bg-green-500/10 dark:border-brand-500/30 dark:bg-brand-500/10 shadow-sm mt-1">
              <div className="flex items-center gap-1.5">
                <span className="relative flex size-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                  <span className="relative inline-flex rounded-full size-2 bg-green-500"></span>
                </span>
                <span className="text-xs font-semibold text-green-700 dark:text-brand-300">
                  Update available: v{updateInfo.latestVersion}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setShowUpdateModal(true)}
                  className="px-2.5 py-1 rounded-lg bg-green-600 hover:bg-green-700 dark:bg-brand-500 dark:hover:bg-brand-600 text-white text-[11px] font-semibold transition-all shadow-sm active:scale-95 cursor-pointer"
                >
                  Update now
                </button>
                <button
                  onClick={() => copy(INSTALL_CMD)}
                  title="Copy install command"
                  className="flex-1 text-left hover:opacity-80 transition-opacity cursor-pointer min-w-0"
                >
                  <code className="block text-[10px] text-green-700 dark:text-brand-300 font-mono truncate">
                    {copied ? "✓ copied!" : INSTALL_CMD}
                  </code>
                </button>
              </div>
            </div>
          )}
        </div>

        {/* Navigation */}
        <nav className="flex-1 px-3 py-2 space-y-1 overflow-y-auto custom-scrollbar">
          {navItems.map((item) => {
            const active = isActive(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                onClick={onClose}
                className={cn(
                  "relative flex items-center gap-3 px-3 h-9 rounded-xl transition-all duration-150 group",
                  active
                    ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 font-semibold shadow-sm before:absolute before:left-0 before:top-2 before:bottom-2 before:w-1 before:rounded-r-full before:bg-brand-500"
                    : "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
                )}
              >
                <span
                  className={cn(
                    "material-symbols-outlined text-[19px] shrink-0 transition-transform duration-150 group-hover:scale-105",
                    active ? "fill-1 text-brand-500 dark:text-brand-400" : "group-hover:text-brand-500"
                  )}
                >
                  {item.icon}
                </span>
                <span className="text-[13px] tracking-tight">{item.label}</span>
              </Link>
            );
          })}

          {/* System section */}
          <div className="pt-3 mt-3 border-t border-border/50 space-y-1">
            <p className="px-3 text-[10px] font-bold tracking-widest text-text-muted/60 uppercase mb-1.5">
              System
            </p>

            {/* Media Providers accordion */}
            <button
              onClick={() => setMediaOpen((v) => !v)}
              className={cn(
                "w-full flex items-center gap-3 px-3 h-9 rounded-xl transition-all duration-150 group cursor-pointer",
                pathname.startsWith("/dashboard/media-providers")
                  ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 font-semibold"
                  : "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
              )}
            >
              <span className="material-symbols-outlined text-[19px] shrink-0">perm_media</span>
              <span className="text-[13px] tracking-tight flex-1 text-left">Media Providers</span>
              <span
                className="material-symbols-outlined text-[16px] transition-transform duration-200"
                style={{ transform: mediaOpen ? "rotate(180deg)" : "rotate(0deg)" }}
              >
                expand_more
              </span>
            </button>
            {mediaOpen && (
              <div className="pl-3 space-y-0.5 border-l border-border/60 ml-5 my-1">
                {MEDIA_PROVIDER_KINDS.filter((k) => VISIBLE_MEDIA_KINDS.includes(k.id)).map((kind) => {
                  const subActive = pathname.startsWith(`/dashboard/media-providers/${kind.id}`);
                  return (
                    <Link
                      key={kind.id}
                      href={`/dashboard/media-providers/${kind.id}`}
                      onClick={onClose}
                      className={cn(
                        "flex items-center gap-2.5 px-3 h-8 rounded-lg transition-all text-xs",
                        subActive
                          ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 font-medium"
                          : "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
                      )}
                    >
                      <span className="material-symbols-outlined text-[16px] shrink-0">{kind.icon}</span>
                      <span className="truncate">{kind.label}</span>
                    </Link>
                  );
                })}
                <Link
                  key={COMBINED_WEB_ITEM.id}
                  href={COMBINED_WEB_ITEM.href}
                  onClick={onClose}
                  className={cn(
                    "flex items-center gap-2.5 px-3 h-8 rounded-lg transition-all text-xs",
                    pathname.startsWith(COMBINED_WEB_ITEM.href)
                      ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 font-medium"
                      : "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
                  )}
                >
                  <span className="material-symbols-outlined text-[16px] shrink-0">{COMBINED_WEB_ITEM.icon}</span>
                  <span className="truncate">{COMBINED_WEB_ITEM.label}</span>
                </Link>
              </div>
            )}

            {systemItems.map((item) => {
              const active = isActive(item.href);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={onClose}
                  className={cn(
                    "relative flex items-center gap-3 px-3 h-9 rounded-xl transition-all duration-150 group",
                    active
                      ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 font-semibold shadow-sm before:absolute before:left-0 before:top-2 before:bottom-2 before:w-1 before:rounded-r-full before:bg-brand-500"
                      : "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
                  )}
                >
                  <span
                    className={cn(
                      "material-symbols-outlined text-[19px] shrink-0 transition-transform duration-150 group-hover:scale-105",
                      active ? "fill-1 text-brand-500 dark:text-brand-400" : "group-hover:text-brand-500"
                    )}
                  >
                    {item.icon}
                  </span>
                  <span className="text-[13px] tracking-tight">{item.label}</span>
                </Link>
              );
            })}

            {/* Debug items */}
            {debugItems.map((item) => {
              const show = item.href !== "/dashboard/translator" || enableTranslator;
              if (!show) return null;
              const active = isActive(item.href);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={onClose}
                  className={cn(
                    "relative flex items-center gap-3 px-3 h-9 rounded-xl transition-all duration-150 group",
                    active
                      ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 font-semibold shadow-sm before:absolute before:left-0 before:top-2 before:bottom-2 before:w-1 before:rounded-r-full before:bg-brand-500"
                      : "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
                  )}
                >
                  <span
                    className={cn(
                      "material-symbols-outlined text-[19px] shrink-0 transition-transform duration-150 group-hover:scale-105",
                      active ? "fill-1 text-brand-500 dark:text-brand-400" : "group-hover:text-brand-500"
                    )}
                  >
                    {item.icon}
                  </span>
                  <span className="text-[13px] tracking-tight">{item.label}</span>
                </Link>
              );
            })}

            {/* Remote */}
            <a
              href="https://9remote.cc"
              target="_blank"
              rel="noreferrer"
              onClick={onClose}
              className={cn(
                "flex items-center gap-3 px-3 h-9 rounded-xl transition-all duration-150 group w-full",
                "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
              )}
            >
              <span className="material-symbols-outlined text-[19px] shrink-0 group-hover:text-brand-500 transition-colors">
                computer
              </span>
              <span className="text-[13px] tracking-tight font-medium">9Remote</span>
              <span className="ml-auto text-[10px] font-bold px-1.5 py-0.5 rounded-full bg-orange-500/15 text-orange-500 border border-orange-500/20">
                HOT
              </span>
            </a>

            {/* 9English */}
            <a
              href="https://9english.net/"
              target="_blank"
              rel="noreferrer"
              onClick={onClose}
              className={cn(
                "flex items-center gap-3 px-3 h-9 rounded-xl transition-all duration-150 group w-full",
                "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
              )}
            >
              <span className="material-symbols-outlined text-[19px] shrink-0 group-hover:text-brand-500 transition-colors">
                translate
              </span>
              <span className="text-[13px] tracking-tight font-medium">9English</span>
            </a>

            {/* Settings */}
            <Link
              href="/dashboard/profile"
              onClick={onClose}
              className={cn(
                "relative flex items-center gap-3 px-3 h-9 rounded-xl transition-all duration-150 group",
                isActive("/dashboard/profile")
                  ? "bg-brand-500/10 text-brand-600 dark:text-brand-400 font-semibold shadow-sm before:absolute before:left-0 before:top-2 before:bottom-2 before:w-1 before:rounded-r-full before:bg-brand-500"
                  : "text-text-muted hover:bg-surface-2/70 hover:text-text-main"
              )}
            >
              <span
                className={cn(
                  "material-symbols-outlined text-[19px] shrink-0 transition-transform duration-150 group-hover:scale-105",
                  isActive("/dashboard/profile") ? "fill-1 text-brand-500 dark:text-brand-400" : "group-hover:text-brand-500"
                )}
              >
                settings
              </span>
              <span className="text-[13px] tracking-tight">Settings</span>
            </Link>
          </div>
        </nav>

      </aside>


      {/* Update Confirmation Modal */}
      <ConfirmModal
        isOpen={showUpdateModal}
        onClose={() => setShowUpdateModal(false)}
        onConfirm={handleUpdate}
        title="Update 9Router"
        message={`Show install command for v${updateInfo?.latestVersion || ""}? You can copy it and shutdown to install manually.`}
        confirmText="Show Command"
        cancelText="Cancel"
        variant="primary"
      />

      {/* Disconnected / Updating Overlay */}
      {(isDisconnected || isUpdating) && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-6">
          {isUpdating ? (
            <ManualUpdatePanel
              latestVersion={updateInfo?.latestVersion}
              installCmd={INSTALL_CMD}
              copied={copied}
              onCopyAndShutdown={handleCopyAndShutdown}
              onCancel={handleCancelUpdate}
              countdown={shutdownCountdown}
              isDisconnected={isDisconnected}
            />
          ) : (
            <div className="text-center p-8">
              <div className="flex items-center justify-center size-16 rounded-full bg-red-500/20 text-red-500 mx-auto mb-4">
                <span className="material-symbols-outlined text-[32px]">power_off</span>
              </div>
              <h2 className="text-xl font-semibold text-white mb-2">Server Disconnected</h2>
              <p className="text-text-muted mb-6">The proxy server has been stopped.</p>
              <Button variant="secondary" onClick={() => globalThis.location.reload()}>
                Reload Page
              </Button>
            </div>
          )}
        </div>
      )}
    </>
  );
}

Sidebar.propTypes = {
  onClose: PropTypes.func,
};

function ManualUpdatePanel({ latestVersion, installCmd, copied, onCopyAndShutdown, onCancel, countdown, isDisconnected }) {
  const isCountingDown = countdown > 0;
  return (
    <div className="w-full max-w-lg rounded-xl bg-neutral-900/95 border border-white/10 p-6 text-white">
      <div className="flex items-center gap-3 mb-4">
        <div className="flex items-center justify-center size-11 rounded-full bg-amber-500/20 text-amber-400">
          <span className="material-symbols-outlined text-[24px]">content_copy</span>
        </div>
        <div>
          <h2 className="text-lg font-semibold">Update 9Router{latestVersion ? ` to v${latestVersion}` : ""}</h2>
          <p className="text-xs text-white/60">
            {isDisconnected
              ? "Server stopped. Paste the command into a terminal to install."
              : isCountingDown
                ? `Command copied. Server will stop in ${countdown}s...`
                : "Click the button below to copy the install command and shutdown."}
          </p>
        </div>
      </div>

      <p className="text-sm text-white/80 mb-2">Install command:</p>
      <div className="w-full px-3 py-2 rounded bg-white/5 mb-4">
        <code className="text-xs font-mono text-amber-400 break-all">{installCmd}</code>
      </div>

      <ol className="text-xs text-white/70 space-y-1 list-decimal list-inside mb-4">
        <li>Click <strong>Copy & Shutdown</strong> below.</li>
        <li>Paste the command into your terminal and press Enter.</li>
        <li>Run <code className="px-1 rounded bg-white/10 text-green-400">9router</code> again after install.</li>
      </ol>

      {isDisconnected ? (
        <Button variant="secondary" fullWidth onClick={() => globalThis.location.reload()}>
          Reload Page
        </Button>
      ) : (
        <div className="flex gap-2">
          <Button variant="secondary" onClick={onCancel} disabled={isCountingDown}>
            Cancel
          </Button>
          <Button variant="primary" fullWidth onClick={onCopyAndShutdown} disabled={isCountingDown}>
            {copied ? "✓ Copied — shutting down..." : isCountingDown ? `Shutting down in ${countdown}s` : "Copy & Shutdown"}
          </Button>
        </div>
      )}
    </div>
  );
}

ManualUpdatePanel.propTypes = {
  latestVersion: PropTypes.string,
  installCmd: PropTypes.string.isRequired,
  copied: PropTypes.bool,
  onCopyAndShutdown: PropTypes.func.isRequired,
  onCancel: PropTypes.func.isRequired,
  countdown: PropTypes.number,
  isDisconnected: PropTypes.bool,
};
