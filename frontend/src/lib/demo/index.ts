// Demo bootstrap (ssl-domain-exporter): the app uses axios, so we install a custom
// axios ADAPTER at module-evaluation time — which must happen BEFORE api/client.ts
// runs axios.create(), so this module is imported FIRST in main.tsx.
/* eslint-disable @typescript-eslint/no-explicit-any */
import axios from "axios";
import { demoAdapter } from "./server";
import { getDB } from "./db";

export { resetDemo } from "./db";

export function isDemo(): boolean {
  try { const env = (import.meta as any).env; if (env && env.VITE_DEMO === "1") return true; } catch { /* */ }
  return typeof window !== "undefined" && (window as any).__DEMO__ === true;
}

if (isDemo()) {
  (window as any).__DEMO__ = true;
  // Route ALL axios traffic (global + every instance created afterwards) to the mock.
  (axios.defaults as any).adapter = demoAdapter;
  getDB();
}

export function installDemo() {
  if (isDemo()) (axios.defaults as any).adapter = demoAdapter;
}
