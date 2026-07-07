"use client";
import { useEffect, useState } from "react";
import { useI18n } from "@/i18n/provider";
import { api, getAdminToken, setAdminToken, type AuditEntry, type BanEntry, type UsageEntry } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { formatRelative, formatTime, cn } from "@/lib/utils";
import { ShieldCheck, Lock, Unlock, Activity, Database, RefreshCw, KeyRound } from "lucide-react";

export default function AdminPage() {
  const { t } = useI18n();
  const [ready, setReady] = useState(!!getAdminToken());
  const [bans, setBans] = useState<BanEntry[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [usage, setUsage] = useState<UsageEntry[]>([]);
  const [tab, setTab] = useState<"bans" | "audit" | "usage">("bans");
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const [b, a, u] = await Promise.all([api.listBans(), api.listAudit(), api.listUsage()]);
      setBans(b); setAudit(a); setUsage(u);
    } catch (e: any) {
      alert(e.message);
      if (String(e.message).includes("admin")) {
        setReady(false);
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { if (ready) load(); }, [ready]);

  if (!ready) return <AdminLogin onOk={() => setReady(true)} />;

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border p-2 gap-2">
        <div className="flex items-center gap-2 text-sm font-medium">
          <ShieldCheck size={14} className="text-muted" />
          {t("admin.title")}
        </div>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => { setAdminToken(""); setReady(false); }} className="text-xs text-muted gap-1.5">
            <KeyRound size={12} /> reset token
          </Button>
          <Button variant="ghost" size="icon" onClick={load} aria-label="refresh">
            <RefreshCw size={12} className={cn(loading && "animate-spin")} />
          </Button>
        </div>
      </div>

      <div className="flex border-b border-border">
        {[
          { id: "bans" as const, label: t("admin.bans"), icon: Lock },
          { id: "audit" as const, label: t("admin.audit"), icon: Activity },
          { id: "usage" as const, label: t("admin.usage"), icon: Database },
        ].map((x) => (
          <button
            key={x.id}
            onClick={() => setTab(x.id)}
            className={cn(
              "flex items-center gap-1.5 px-3 py-2 text-xs border-b-2 transition-colors",
              tab === x.id ? "border-fg text-fg" : "border-transparent text-muted hover:text-fg"
            )}
          >
            <x.icon size={12} />
            {x.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-3">
        {tab === "bans" && <BansTable bans={bans} onUnban={load} />}
        {tab === "audit" && <AuditTable audit={audit} />}
        {tab === "usage" && <UsageTable usage={usage} />}
      </div>
    </div>
  );
}

function AdminLogin({ onOk }: { onOk: () => void }) {
  const [tok, setTok] = useState("");
  const { t } = useI18n();
  return (
    <div className="flex h-full items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <KeyRound size={14} /> Admin token
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <Input
              type="password"
              value={tok}
              onChange={(e) => setTok(e.target.value)}
              placeholder="OMNI_ADMIN_TOKEN"
            />
            <Button
              className="w-full"
              onClick={() => {
                setAdminToken(tok.trim());
                onOk();
              }}
            >
              Enter
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function BansTable({ bans, onUnban }: { bans: BanEntry[]; onUnban: () => void }) {
  const { t } = useI18n();
  if (bans.length === 0) return <p className="text-xs text-muted text-center pt-6">{t("admin.no_bans")}</p>;
  return (
    <table className="w-full text-xs">
      <thead className="text-muted text-left">
        <tr>
          <th className="font-medium pb-2">Key</th>
          <th className="font-medium pb-2">Fails</th>
          <th className="font-medium pb-2">First fail</th>
          <th className="font-medium pb-2">Banned until</th>
          <th className="font-medium pb-2"></th>
        </tr>
      </thead>
      <tbody className="font-mono">
        {bans.map((b) => (
          <tr key={b.key} className="border-t border-border">
            <td className="py-1.5">{b.key}</td>
            <td className="py-1.5">{b.fails}</td>
            <td className="py-1.5 text-muted">{formatRelative(b.first_fail)}</td>
            <td className="py-1.5">
              {b.banned_until > 0 ? (
                <span className={b.expired ? "text-muted" : "text-red-500"}>
                  {b.expired ? "expired" : formatTime(b.banned_until)}
                </span>
              ) : (
                <span className="text-muted">—</span>
              )}
            </td>
            <td className="py-1.5 text-right">
              <Button
                size="sm"
                variant="ghost"
                className="text-xs gap-1"
                onClick={async () => { await api.unban(b.key); onUnban(); }}
              >
                <Unlock size={10} />
                {t("admin.unban")}
              </Button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function AuditTable({ audit }: { audit: AuditEntry[] }) {
  return (
    <table className="w-full text-xs">
      <thead className="text-muted text-left">
        <tr>
          <th className="font-medium pb-2">When</th>
          <th className="font-medium pb-2">Channel</th>
          <th className="font-medium pb-2">Remote</th>
          <th className="font-medium pb-2">Action</th>
          <th className="font-medium pb-2">Detail</th>
          <th className="font-medium pb-2">OK</th>
        </tr>
      </thead>
      <tbody className="font-mono">
        {audit.map((a, i) => (
          <tr key={i} className="border-t border-border">
            <td className="py-1.5 text-muted">{formatRelative(a.ts)}</td>
            <td className="py-1.5">{a.channel}</td>
            <td className="py-1.5 text-muted">{a.remote_id}</td>
            <td className="py-1.5">{a.action}</td>
            <td className="py-1.5 text-muted max-w-xs truncate">{a.detail}</td>
            <td className={a.ok ? "text-green-500" : "text-red-500"}>{a.ok ? "✓" : "✗"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function UsageTable({ usage }: { usage: UsageEntry[] }) {
  if (usage.length === 0) return <p className="text-xs text-muted text-center pt-6">No usage yet</p>;
  return (
    <table className="w-full text-xs">
      <thead className="text-muted text-left">
        <tr>
          <th className="font-medium pb-2">Day</th>
          <th className="font-medium pb-2">Agent</th>
          <th className="font-medium pb-2 text-right">Requests</th>
          <th className="font-medium pb-2 text-right">Tokens in</th>
          <th className="font-medium pb-2 text-right">Tokens out</th>
        </tr>
      </thead>
      <tbody className="font-mono">
        {usage.map((u, i) => (
          <tr key={i} className="border-t border-border">
            <td className="py-1.5">{u.day}</td>
            <td className="py-1.5">{u.agent}</td>
            <td className="py-1.5 text-right">{u.requests}</td>
            <td className="py-1.5 text-right">{u.tokens_in.toLocaleString()}</td>
            <td className="py-1.5 text-right">{u.tokens_out.toLocaleString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
