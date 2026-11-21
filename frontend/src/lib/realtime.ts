import { useEffect, useRef } from 'react';

/** One change signal from /api/v1/realtime; it never carries data, only what changed. */
export type RealtimeSignal = { t?: string; schedule_id?: string; program_id?: string; regency_id?: string; user_id?: string };
type Listener = (signal: RealtimeSignal) => void;

const reconnectDelays = [1_000, 3_000, 10_000, 30_000];
const listeners = new Set<Listener>();
let socket: WebSocket | null = null;
let timer = 0;
let attempts = 0;

/** One WebSocket for the whole page, opened while someone listens and reconnecting with backoff. */
function connect() {
  if (typeof WebSocket === 'undefined' || socket) return;
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const current = new WebSocket(`${protocol}//${window.location.host}/api/v1/realtime`);
  socket = current;
  current.onopen = () => { attempts = 0; };
  current.onmessage = (event: MessageEvent<string>) => {
    let signal: RealtimeSignal;
    try { signal = JSON.parse(event.data) as RealtimeSignal; } catch { return; }
    listeners.forEach((listener) => listener(signal));
  };
  current.onclose = () => {
    if (socket === current) socket = null;
    if (listeners.size === 0) return;
    timer = window.setTimeout(connect, reconnectDelays[Math.min(attempts, reconnectDelays.length - 1)]);
    attempts += 1;
  };
}

export function subscribeRealtime(listener: Listener): () => void {
  listeners.add(listener);
  connect();
  return () => {
    listeners.delete(listener);
    if (listeners.size > 0) return;
    window.clearTimeout(timer);
    const current = socket;
    socket = null;
    current?.close();
  };
}

/** Calls onSignal for every realtime signal while the component is mounted. */
export function useRealtimeSignals(onSignal: Listener) {
  const handler = useRef(onSignal);
  useEffect(() => { handler.current = onSignal; });
  useEffect(() => subscribeRealtime((signal) => handler.current(signal)), []);
}
