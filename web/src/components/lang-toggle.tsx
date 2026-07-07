"use client";
import { useI18n } from "@/i18n/provider";
import { Button } from "@/components/ui/button";
import { Languages } from "lucide-react";

export function LangToggle() {
  const { lang, setLang } = useI18n();
  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => setLang(lang === "vi" ? "en" : "vi")}
      className="gap-1.5 font-mono text-xs"
    >
      <Languages size={14} />
      {lang === "vi" ? "EN" : "VN"}
    </Button>
  );
}
