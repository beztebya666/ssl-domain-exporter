// In-browser DB for the demo build — per-browser localStorage sandbox + Reset.
/* eslint-disable @typescript-eslint/no-explicit-any */
import { buildSeed } from "./seed";

export type DemoDB = ReturnType<typeof buildSeed>;
const KEY = "ssl-domain-exporter:demo:v1";
let db: DemoDB | null = null;

function load(): DemoDB {
  try { const raw = localStorage.getItem(KEY); if (raw) return JSON.parse(raw) as DemoDB; } catch { /* */ }
  const fresh = buildSeed();
  try { localStorage.setItem(KEY, JSON.stringify(fresh)); } catch { /* */ }
  return fresh;
}
export function getDB(): DemoDB { if (!db) db = load(); return db; }
export function saveDB() { try { localStorage.setItem(KEY, JSON.stringify(db)); } catch { /* */ } }
export function resetDemo() { db = buildSeed(); saveDB(); }
