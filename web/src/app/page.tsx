"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useI18n } from "@/i18n/provider";
import { api, getAdminToken, type AgentInfo, type PublicInfo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Turnstile } from "@/components/turnstile";
import { LangToggle } from "@/components/lang-toggle";
import { ThemeToggle } from "@/components/theme-toggle";
import { AlertCircle, Loader2, ShieldCheck } from "lucide-react";

export default function LoginPage() {
  const { t } = useI18n();
  const router = useRouter();
  const [info, setInfo] = useState<PublicInfo | null>(null);
  const [token, setToken] = useState("");
  const [turnstileToken, setTurnstileToken] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.getPublicInfo().then(setInfo).catch(() => {});
    // already logged in? skip to app
    if (typeof window !== "undefined" && localStorage.getItem("omni.token")) {
      router.replace("/app");
    }
  }, [router]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await api.login(token.trim(), turnstileToken);
      router.replace("/app");
    } catch (err: any) {
      const msg = String(err.message || err);
      if (msg.includes("banned")) setError(t("login.banned"));
      else setError(msg);
      setTurnstileToken("");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="absolute top-4 right-4 flex items-center gap-1">
        <LangToggle />
        <ThemeToggle />
      </div>

      <div className="w-full max-w-sm animate-slide-up">
        {/* Logo / brand */}
        <div className="mb-8 flex flex-col items-center text-center">
          <OmniLogo className="h-10 w-10 mb-3" />
          <h1 className="text-xl font-semibold tracking-tight">{t("app.name")}</h1>
          <p className="text-xs text-muted mt-1">{t("app.tagline")}</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck size={14} className="text-muted" />
              {t("login.title")}
            </CardTitle>
            <p className="text-xs text-muted">{t("login.subtitle")}</p>
          </CardHeader>
          <CardContent>
            <form onSubmit={submit} className="space-y-3">
              <div className="space-y-1.5">
                <label className="text-xs font-medium">{t("login.token")}</label>
                <Input
                  type="password"
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  placeholder={t("login.token.placeholder")}
                  required
                  autoComplete="off"
                  autoFocus
                />
              </div>

              {info?.turnstile_enabled && info?.turnstile_site_key && (
                <div className="space-y-1.5">
                  <label className="text-xs font-medium">{t("login.verify")}</label>
                  <Turnstile siteKey={info.turnstile_site_key} onToken={setTurnstileToken} />
                </div>
              )}

              {error && (
                <div className="flex items-start gap-2 text-xs text-red-500 bg-red-500/10 border border-red-500/20 rounded p-2">
                  <AlertCircle size={14} className="mt-0.5 shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              <Button type="submit" className="w-full" disabled={loading || !token}>
                {loading ? <Loader2 size={14} className="animate-spin" /> : null}
                {t("login.submit")}
              </Button>
            </form>
          </CardContent>
        </Card>

        {/* Agent list footer */}
        {info?.agents?.length ? (
          <div className="mt-6 flex flex-wrap items-center justify-center gap-1.5">
            {info.agents.map((a: AgentInfo) => (
              <span key={a.id} className="text-[10px] text-muted border border-border rounded px-1.5 py-0.5 font-mono">
                {a.name}
              </span>
            ))}
          </div>
        ) : null}

        <p className="mt-6 text-center text-[10px] text-muted">
          OmniAgent · Token + Cloudflare Turnstile · progressive ban
        </p>
      </div>
    </div>
  );
}

// Minimal mono "O" logo
function OmniLogo({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="20" cy="20" r="18" stroke="currentColor" strokeWidth="1.5" />
      <circle cx="20" cy="20" r="6" fill="currentColor" />
      <path d="M2 20 H8 M32 20 H38" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}
