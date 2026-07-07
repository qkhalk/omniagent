"use client";
import { useEffect, useState } from "react";
import { useI18n } from "@/i18n/provider";
import { api, type FileEntry } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/input";
import { cn, formatBytes, formatRelative } from "@/lib/utils";
import { ChevronRight, Folder, File as FileIcon, Save, ArrowLeft, RefreshCw } from "lucide-react";

export default function FilesPage() {
  const { t } = useI18n();
  const [path, setPath] = useState("");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [file, setFile] = useState<{ path: string; content: string; dirty: boolean } | null>(null);

  async function load(p: string) {
    setLoading(true);
    try {
      const r = await api.listFiles(p);
      setEntries(r.entries || []);
      setPath(p);
    } catch (e: any) {
      alert(e.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(""); }, []);

  function open(entry: FileEntry) {
    const next = path ? `${path}/${entry.name}` : entry.name;
    if (entry.is_dir) {
      load(next);
      return;
    }
    api.readFile(next).then((r) => setFile({ path: next, content: r.content, dirty: false }));
  }

  function up() {
    if (!path) return;
    const i = path.lastIndexOf("/");
    load(i >= 0 ? path.slice(0, i) : "");
  }

  async function save() {
    if (!file) return;
    try {
      await api.writeFile(file.path, file.content);
      setFile({ ...file, dirty: false });
    } catch (e: any) {
      alert(e.message);
    }
  }

  const crumbs = path ? path.split("/") : [];

  return (
    <div className="flex h-full">
      {/* Explorer */}
      <div className="flex-1 flex flex-col min-w-0 border-r border-border">
        <div className="flex items-center justify-between border-b border-border p-2 gap-2">
          <div className="flex items-center gap-1 text-xs font-mono min-w-0">
            <button
              onClick={() => load("")}
              className="text-muted hover:text-fg"
            >
              workspace
            </button>
            {crumbs.map((c, i) => (
              <span key={i} className="flex items-center gap-1">
                <ChevronRight size={12} className="text-muted" />
                <button
                  onClick={() => load(crumbs.slice(0, i + 1).join("/"))}
                  className="hover:text-fg"
                >
                  {c}
                </button>
              </span>
            ))}
          </div>
          <div className="flex items-center gap-1">
            {path && (
              <Button variant="ghost" size="sm" onClick={up} className="gap-1 text-xs">
                <ArrowLeft size={12} /> up
              </Button>
            )}
            <Button variant="ghost" size="icon" onClick={() => load(path)} aria-label="refresh">
              <RefreshCw size={12} />
            </Button>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-1.5">
          {loading && <p className="text-xs text-muted p-3">{t("common.loading")}</p>}
          {!loading && entries.length === 0 && (
            <p className="text-xs text-muted p-3 text-center">{t("files.empty")}</p>
          )}
          {entries.map((e, i) => (
            <button
              key={i}
              onClick={() => open(e)}
              className="w-full flex items-center gap-2 px-2 py-1 rounded text-sm hover:bg-card text-left"
            >
              {e.is_dir ? <Folder size={14} className="text-muted" /> : <FileIcon size={14} className="text-muted" />}
              <span className="flex-1 truncate">{e.name}</span>
              {!e.is_dir && <span className="text-[10px] text-muted">{formatBytes(e.size)}</span>}
              <span className="text-[10px] text-muted">{formatRelative(e.mod)}</span>
            </button>
          ))}
        </div>
      </div>

      {/* Editor */}
      {file && (
        <div className="w-1/2 flex flex-col">
          <div className="flex items-center justify-between border-b border-border p-2 gap-2">
            <div className="flex items-center gap-2 text-xs font-mono min-w-0">
              <FileIcon size={12} className="text-muted" />
              <span className="truncate">{file.path}</span>
              {file.dirty && <span className="text-orange-500">●</span>}
            </div>
            <Button size="sm" onClick={save} disabled={!file.dirty} className="gap-1.5 text-xs">
              <Save size={12} />
              {file.dirty ? t("files.save") : t("files.saved")}
            </Button>
          </div>
          <Textarea
            value={file.content}
            onChange={(e) => setFile({ ...file, content: e.target.value, dirty: true })}
            className="flex-1 rounded-none border-0 resize-none text-xs"
            spellCheck={false}
          />
        </div>
      )}
    </div>
  );
}
