import { useState } from "react";
import { resetDemo } from "../lib/demo";

/** Floating demo control (demo build only): private in-browser sandbox + Reset.
 *  Inline-styled so it renders identically regardless of the app's CSS. */
export function DemoBanner() {
  const [busy, setBusy] = useState(false);
  const reset = () => { setBusy(true); resetDemo(); setTimeout(() => location.reload(), 150); };
  const wrap: React.CSSProperties = { position: "fixed", bottom: 16, right: 16, zIndex: 9999, display: "flex", alignItems: "center", gap: 12, borderRadius: 12, border: "1px solid rgba(113,113,122,.6)", background: "rgba(24,24,27,.96)", color: "#fafafa", padding: "10px 16px", fontSize: 13, fontFamily: "system-ui, sans-serif", boxShadow: "0 10px 30px rgba(0,0,0,.5)" };
  const dot: React.CSSProperties = { width: 9, height: 9, borderRadius: 9, background: "#34d399", display: "inline-block", boxShadow: "0 0 0 3px rgba(52,211,153,.25)" };
  const btn: React.CSSProperties = { borderRadius: 8, background: "#10b981", color: "#06241a", fontWeight: 600, border: "none", padding: "6px 12px", cursor: "pointer", opacity: busy ? 0.6 : 1 };
  return (
    <div style={wrap}>
      <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span style={dot} /><strong>Live demo</strong>
        <span style={{ color: "#a1a1aa" }}>· your private in-browser sandbox</span>
      </span>
      <button style={btn} onClick={reset} disabled={busy} title="Wipe your changes and restore the seeded demo data">{busy ? "Resetting…" : "Reset demo"}</button>
      <a style={{ color: "#a1a1aa", textDecoration: "none" }} href="https://github.com/beztebya666/ssl-domain-exporter" target="_blank" rel="noreferrer">GitHub ↗</a>
    </div>
  );
}
