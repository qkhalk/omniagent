"use client";
import { useEffect, useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { useI18n } from "@/i18n/provider";
import { api, getToken } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { LangToggle } from "@/components/lang-toggle";
import { ThemeToggle } from "@/components/theme-toggle";
import { LogOut, MessageSquare, FolderTree, ShieldCheck, X } from "lucide-react";

export default function AppShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { t } = useI18n();
  const [ready, setReady] = useState(false);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  useEffect(() => {
    if (!getToken()) {
      router.replace("/");
      return;
    }
    // verify token still valid
    api.me()
      .then(() => setReady(true))
      .catch(() => router.replace("/"));
  }, [router]);

  if (!ready) {
    return (
      <div className="min-h-screen flex items-center justify-center text-xs text-muted">…</div>
    );
  }

  const nav = [
    { href: "/app", label: t("app.chat"), icon: MessageSquare },
    { href: "/app/files", label: t("app.files"), icon: FolderTree },
    { href: "/app/admin", label: t("app.admin"), icon: ShieldCheck },
  ];

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Sidebar (desktop) */}
      <aside className="hidden md:flex w-56 shrink-0 border-r border-border flex-col">
        <Brand />
        <nav className="flex-1 p-2 space-y-0.5">
          {nav.map((n) => (
            <Link
              key={n.href}
              href={n.href}
              className={`flex items-center gap-2 px-2.5 py-1.5 rounded text-sm transition-colors ${
                pathname === n.href ? "bg-card" : "hover:bg-card text-muted"
              }`}
            >
              <n.icon size={14} />
              {n.label}
            </Link>
          ))}
        </nav>
        <div className="p-2 border-t border-border flex items-center justify-between">
          <LangToggle />
          <ThemeToggle />
          <Button
            variant="ghost"
            size="sm"
            onClick={async () => { await api.logout(); router.replace("/"); }}
            className="gap-1.5 text-xs text-muted"
          >
            <LogOut size={14} />
            {t("app.logout")}
          </Button>
        </div>
      </aside>

      {/* Mobile nav drawer */}
      {mobileNavOpen && (
        <div className="md:hidden fixed inset-0 z-50 bg-black/50" onClick={() => setMobileNavOpen(false)}>
          <div
            className="absolute left-0 top-0 bottom-0 w-56 bg-bg border-r border-border flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between p-2 border-b border-border">
              <Brand />
              <Button variant="ghost" size="icon" onClick={() => setMobileNavOpen(false)}>
                <X size={16} />
              </Button>
            </div>
            <nav className="flex-1 p-2 space-y-0.5">
              {nav.map((n) => (
                <Link
                  key={n.href}
                  href={n.href}
                  onClick={() => setMobileNavOpen(false)}
                  className={`flex items-center gap-2 px-2.5 py-1.5 rounded text-sm ${
                    pathname === n.href ? "bg-card" : "hover:bg-card text-muted"
                  }`}
                >
                  <n.icon size={14} />
                  {n.label}
                </Link>
              ))}
            </nav>
            <div className="p-2 border-t border-border flex items-center gap-1">
              <LangToggle />
              <ThemeToggle />
              <Button
                variant="ghost"
                size="sm"
                onClick={async () => { await api.logout(); router.replace("/"); }}
                className="gap-1.5 text-xs text-muted ml-auto"
              >
                <LogOut size={14} />
                {t("app.logout")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Main content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Mobile top bar */}
        <div className="md:hidden flex items-center justify-between border-b border-border p-2">
          <Button variant="ghost" size="icon" onClick={() => setMobileNavOpen(true)} aria-label="menu">
            <FolderTree size={16} />
          </Button>
          <Brand compact />
          <div className="w-9" />
        </div>
        <main className="flex-1 overflow-hidden">{children}</main>
      </div>
    </div>
  );
}

function Brand({ compact }: { compact?: boolean }) {
  return (
    <div className={`flex items-center gap-2 ${compact ? "" : "p-3"}`}>
      <svg className="h-5 w-5" viewBox="0 0 40 40" fill="none">
        <circle cx="20" cy="20" r="18" stroke="currentColor" strokeWidth="1.5" />
        <circle cx="20" cy="20" r="6" fill="currentColor" />
        <path d="M2 20 H8 M32 20 H38" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
      </svg>
      <span className="font-semibold tracking-tight text-sm">OmniAgent</span>
    </div>
  );
}
