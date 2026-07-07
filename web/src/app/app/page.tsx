"use client";
import { useEffect, useRef, useState, useCallback } from "react";
import { useI18n } from "@/i18n/provider";
import { api, type AgentInfo, type Message, type PublicInfo, type Session } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { cn, formatRelative } from "@/lib/utils";
import {
  Plus,
  Send,
  Square,
  Trash2,
  Loader2,
  Terminal,
  ChevronRight,
  X,
  Bot,
  User,
  Wrench,
} from "lucide-react";

interface StreamEvent {
  type: string;
  text?: string;
  tool_name?: string;
  tool_input?: string;
  role?: string;
  content?: string;
}

export default function ChatPage() {
  const { t } = useI18n();
  const [info, setInfo] = useState<PublicInfo | null>(null);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [current, setCurrent] = useState<Session | null>(null);
  const [messages, setMessages] = useState<(Message | StreamEvent)[]>([]);
  const [input, setInput] = useState("");
  const [agent, setAgent] = useState("claude");
  const [project, setProject] = useState("");
  const [projects, setProjects] = useState<string[]>([]);
  const [running, setRunning] = useState(false);
  const [showNewDialog, setShowNewDialog] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const streamBufRef = useRef<HTMLDivElement>(null);

  // Load info + sessions
  useEffect(() => {
    api.getPublicInfo().then((i) => {
      setInfo(i);
      if (i.agents.length) setAgent(i.agents[0].id);
    });
    api.listProjects().then((p) => setProjects(p.dirs || []));
    refreshSessions();
  }, []);

  const refreshSessions = useCallback(async () => {
    try {
      const s = await api.listSessions();
      setSessions(s.filter((x) => !x.closed));
    } catch {}
  }, []);

  // When a session is selected, open WS and load messages
  useEffect(() => {
    if (!current) return;
    setMessages([]);
    setRunning(false);

    // close old ws
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    // load history first
    api.listMessages(current.id).then((m) => {
      setMessages(m.map((x) => ({ type: "replay", role: x.role, content: x.content, tool_name: x.tool_name })));
    });

    const ws = new WebSocket(api.wsUrl(current.id));
    wsRef.current = ws;
    ws.onmessage = (ev) => {
      try {
        const e: StreamEvent = JSON.parse(ev.data);
        setMessages((prev) => [...prev, e]);
        if (e.type === "done") setRunning(false);
        if (e.type === "text" || e.type === "tool") setRunning(true);
      } catch {}
    };
    ws.onclose = () => { wsRef.current = null; };
    return () => { ws.close(); };
  }, [current]);

  // Auto-scroll
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages]);

  async function createSession() {
    try {
      const s = await api.createSession(agent, project, "");
      setCurrent(s);
      setShowNewDialog(false);
      refreshSessions();
    } catch (e: any) {
      alert(e.message);
    }
  }

  async function send() {
    if (!current || !input.trim() || running) return;
    const text = input.trim();
    setInput("");
    setRunning(true);
    try {
      await api.send(current.id, text);
    } catch (e: any) {
      setRunning(false);
      alert(e.message);
    }
  }

  async function cancel() {
    if (!current) return;
    await api.cancel(current.id);
    setRunning(false);
  }

  async function closeSession(id: string) {
    await api.closeSession(id);
    if (current?.id === id) setCurrent(null);
    refreshSessions();
  }

  return (
    <div className="flex h-full">
      {/* Sessions sidebar */}
      <div className="hidden md:flex w-60 shrink-0 border-r border-border flex-col">
        <div className="p-2 border-b border-border">
          <Button size="sm" className="w-full gap-1.5" onClick={() => setShowNewDialog(true)}>
            <Plus size={14} />
            {t("app.new")}
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto p-1.5 space-y-0.5">
          {sessions.length === 0 && (
            <p className="text-xs text-muted p-3 text-center">{t("app.empty")}</p>
          )}
          {sessions.map((s) => (
            <div
              key={s.id}
              className={cn(
                "group flex items-center gap-1.5 rounded px-2 py-1.5 cursor-pointer text-xs",
                current?.id === s.id ? "bg-card" : "hover:bg-card"
              )}
              onClick={() => setCurrent(s)}
            >
              <Bot size={12} className="text-muted shrink-0" />
              <div className="flex-1 min-w-0">
                <div className="truncate font-mono">{s.title || s.id.slice(0, 8)}</div>
                <div className="text-[10px] text-muted">
                  {s.agent} · {formatRelative(s.updated_at)}
                </div>
              </div>
              <button
                className="opacity-0 group-hover:opacity-100 text-muted hover:text-fg"
                onClick={(e) => { e.stopPropagation(); closeSession(s.id); }}
              >
                <Trash2 size={12} />
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Chat area */}
      <div className="flex-1 flex flex-col min-w-0">
        {!current ? (
          <EmptyState onCreate={() => setShowNewDialog(true)} agents={info?.agents || []} />
        ) : (
          <>
            {/* Header */}
            <div className="flex items-center justify-between border-b border-border p-2 gap-2">
              <div className="flex items-center gap-2 text-xs min-w-0">
                <Terminal size={14} className="text-muted shrink-0" />
                <span className="font-mono truncate">
                  {current.title || current.id.slice(0, 8)}
                </span>
                <span className="text-muted">·</span>
                <span className="text-muted">{current.agent}</span>
                <span className="text-muted">·</span>
                <span className="text-muted truncate">{current.project || "(root)"}</span>
              </div>
              <div className="flex items-center gap-1">
                {running && (
                  <Button size="sm" variant="outline" onClick={cancel} className="gap-1.5 text-xs">
                    <Square size={12} className="fill-current" />
                    {t("app.cancel")}
                  </Button>
                )}
              </div>
            </div>

            {/* Messages */}
            <div ref={scrollRef} className="flex-1 overflow-y-auto p-4 space-y-3 font-mono text-sm">
              {messages.length === 0 && (
                <p className="text-xs text-muted text-center pt-8">{t("app.placeholder")}</p>
              )}
              {messages.map((m, i) => (
                <MessageRow key={i} ev={m} />
              ))}
              {running && (
                <div className="text-xs text-muted flex items-center gap-1.5">
                  <Loader2 size={12} className="animate-spin" />
                  agent is working…
                </div>
              )}
            </div>

            {/* Input */}
            <div className="border-t border-border p-2">
              <div className="flex items-end gap-2">
                <Textarea
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      send();
                    }
                  }}
                  placeholder={t("app.placeholder")}
                  className="flex-1 resize-none max-h-32"
                  rows={1}
                  disabled={running}
                />
                <Button onClick={send} disabled={!input.trim() || running} size="icon">
                  {running ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
                </Button>
              </div>
            </div>
          </>
        )}
      </div>

      {/* New session dialog */}
      {showNewDialog && (
        <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4" onClick={() => setShowNewDialog(false)}>
          <div className="bg-bg border border-border rounded w-full max-w-sm p-4 space-y-3" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold">{t("app.new")}</h2>
              <Button variant="ghost" size="icon" onClick={() => setShowNewDialog(false)}>
                <X size={14} />
              </Button>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-medium">{t("app.agent")}</label>
              <Select value={agent} onChange={(e) => setAgent(e.target.value)} className="w-full">
                {(info?.agents || []).map((a: AgentInfo) => (
                  <option key={a.id} value={a.id}>{a.name}</option>
                ))}
              </Select>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-medium">{t("app.project")}</label>
              <Select value={project} onChange={(e) => setProject(e.target.value)} className="w-full">
                <option value="">(workspace root)</option>
                {projects.map((p) => (
                  <option key={p} value={p}>{p}</option>
                ))}
              </Select>
            </div>
            <Button onClick={createSession} className="w-full">
              <Plus size={14} /> {t("app.new")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function MessageRow({ ev }: { ev: Message | StreamEvent }) {
  // StreamEvent shape
  if ("type" in ev && (ev as StreamEvent).type) {
    const e = ev as StreamEvent;
    if (e.type === "replay") {
      if (e.role === "user")
        return (
          <div className="flex gap-2">
            <User size={14} className="text-muted shrink-0 mt-1" />
            <div className="flex-1 whitespace-pre-wrap">{e.content}</div>
          </div>
        );
      if (e.role === "tool")
        return (
          <div className="flex gap-2 text-muted text-xs">
            <Wrench size={12} className="shrink-0 mt-1" />
            <div className="flex-1">{e.tool_name ? `[${e.tool_name}] ` : ""}{e.content}</div>
          </div>
        );
      return (
        <div className="flex gap-2">
          <Bot size={14} className="text-muted shrink-0 mt-1" />
          <div className="flex-1 whitespace-pre-wrap">{e.content}</div>
        </div>
      );
    }
    if (e.type === "text")
      return (
        <div className="flex gap-2">
          <Bot size={14} className="text-muted shrink-0 mt-1" />
          <div className="flex-1 whitespace-pre-wrap">{e.text}</div>
        </div>
      );
    if (e.type === "tool")
      return (
        <div className="flex gap-2 text-xs text-muted pl-4">
          <ChevronRight size={12} className="shrink-0 mt-1" />
          <div className="flex-1">
            <span className="font-semibold text-fg">{e.tool_name}</span>
            {e.tool_input ? `: ${e.tool_input}` : ""}
          </div>
        </div>
      );
    if (e.type === "error")
      return (
        <div className="flex gap-2 text-xs text-red-500">
          <X size={12} className="shrink-0 mt-1" />
          <div className="flex-1 whitespace-pre-wrap">{e.text}</div>
        </div>
      );
    if (e.type === "done") return null;
  }
  return null;
}

function EmptyState({ onCreate, agents }: { onCreate: () => void; agents: AgentInfo[] }) {
  const { t } = useI18n();
  return (
    <div className="flex-1 flex flex-col items-center justify-center p-8 text-center">
      <Terminal size={32} className="text-muted mb-3" />
      <p className="text-sm text-muted mb-4">{t("app.no_session")}</p>
      <Button onClick={onCreate} className="gap-1.5">
        <Plus size={14} /> {t("app.new")}
      </Button>
      {agents.length > 0 && (
        <div className="mt-8 flex flex-wrap gap-1.5 justify-center max-w-sm">
          {agents.map((a) => (
            <span key={a.id} className="text-[10px] text-muted border border-border rounded px-1.5 py-0.5 font-mono">
              {a.name}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
