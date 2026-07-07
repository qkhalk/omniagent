// API client for OmniAgent backend.
const API_BASE =
  (typeof process !== "undefined" && process.env.NEXT_PUBLIC_API_BASE) || "";

const TOKEN_KEY = "omni.token";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(t: string | null) {
  if (typeof window === "undefined") return;
  if (t) localStorage.setItem(TOKEN_KEY, t);
  else localStorage.removeItem(TOKEN_KEY);
}

function headers(): HeadersInit {
  const t = getToken();
  return {
    "Content-Type": "application/json",
    ...(t ? { Authorization: `Bearer ${t}` } : {}),
  };
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { ...headers(), ...(init?.headers || {}) },
    credentials: "include",
  });
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const j = await res.json();
      if (j.error) msg = j.error;
    } catch {}
    throw new Error(msg);
  }
  if (res.status === 204) return null as T;
  return res.json();
}

export interface AgentInfo {
  id: string;
  name: string;
}

export interface PublicInfo {
  agents: AgentInfo[];
  turnstile_site_key: string;
  turnstile_enabled: boolean;
}

export const api = {
  async getPublicInfo(): Promise<PublicInfo> {
    return req("/api/agents");
  },

  async login(token: string, turnstileToken: string): Promise<{ label: string; token: string }> {
    const r = await req<{ label: string; token: string }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ token, turnstile_token: turnstileToken }),
    });
    setToken(r.token);
    return r;
  },

  async me(): Promise<{ label: string; token: string }> {
    return req("/api/auth/me");
  },

  async logout(): Promise<void> {
    try {
      await req("/api/auth/logout", { method: "POST" });
    } catch {}
    setToken(null);
  },

  async listSessions(): Promise<Session[]> {
    return req("/api/sessions");
  },

  async createSession(agent: string, project: string, title?: string): Promise<Session> {
    return req("/api/sessions", {
      method: "POST",
      body: JSON.stringify({ agent, project, title: title || "" }),
    });
  },

  async getSession(id: string): Promise<Session> {
    return req(`/api/sessions/${id}`);
  },

  async closeSession(id: string): Promise<void> {
    await req(`/api/sessions/${id}`, { method: "DELETE" });
  },

  async listMessages(id: string): Promise<Message[]> {
    return req(`/api/sessions/${id}/messages`);
  },

  async send(id: string, prompt: string, model?: string): Promise<void> {
    await req(`/api/sessions/${id}/send`, {
      method: "POST",
      body: JSON.stringify({ prompt, model: model || "" }),
    });
  },

  async cancel(id: string): Promise<void> {
    await req(`/api/sessions/${id}/cancel`, { method: "POST" });
  },

  async listProjects(): Promise<{ workspace: string; recent: string[]; dirs: string[] }> {
    return req("/api/projects");
  },

  async listFiles(path: string): Promise<{ path: string; entries: FileEntry[] }> {
    return req(`/api/files?path=${encodeURIComponent(path)}`);
  },

  async readFile(path: string): Promise<{ path: string; content: string }> {
    return req(`/api/files/read?path=${encodeURIComponent(path)}`);
  },

  async writeFile(path: string, content: string): Promise<void> {
    await req("/api/files/write", {
      method: "POST",
      body: JSON.stringify({ path, content }),
    });
  },

  async listBans(): Promise<BanEntry[]> {
    return req("/api/admin/bans", { headers: { Authorization: `Bearer ${getAdminToken()}` } });
  },

  async unban(key: string): Promise<void> {
    await req(`/api/admin/bans/${encodeURIComponent(key)}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${getAdminToken()}` },
    });
  },

  async listAudit(): Promise<AuditEntry[]> {
    return req("/api/admin/audit", { headers: { Authorization: `Bearer ${getAdminToken()}` } });
  },

  async listUsage(): Promise<UsageEntry[]> {
    return req("/api/admin/usage", { headers: { Authorization: `Bearer ${getAdminToken()}` } });
  },

  wsUrl(sessionId: string): string {
    const tok = getToken() || "";
    const wsBase = API_BASE.replace(/^http/, "ws");
    return `${wsBase}/api/ws?session=${encodeURIComponent(sessionId)}&token=${encodeURIComponent(tok)}`;
  },
};

const ADMIN_TOKEN_KEY = "omni.admin_token";
export function getAdminToken(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem(ADMIN_TOKEN_KEY) || "";
}
export function setAdminToken(t: string) {
  if (typeof window === "undefined") return;
  localStorage.setItem(ADMIN_TOKEN_KEY, t);
}

export interface Session {
  id: string;
  agent: string;
  project: string;
  title: string;
  channel: string;
  created_at: number;
  updated_at: number;
  closed: boolean;
}

export interface Message {
  id: number;
  role: string;
  content: string;
  tool_name?: string;
  created_at: number;
}

export interface FileEntry {
  name: string;
  is_dir: boolean;
  size: number;
  mod: number;
}

export interface BanEntry {
  key: string;
  fails: number;
  first_fail: number;
  banned_until: number;
  expired: boolean;
}

export interface AuditEntry {
  ts: number;
  user_token: string;
  channel: string;
  remote_id: string;
  action: string;
  detail: string;
  ok: boolean;
}

export interface UsageEntry {
  day: string;
  agent: string;
  requests: number;
  tokens_in: number;
  tokens_out: number;
}
