import { useEffect, useRef, useState } from 'react';
import { Pause, Play } from 'lucide-react';
import { Button } from '../../../components/ui/button';
import { dayStart } from './timeline';

export const TIMELINE_STEP_MS = 700;

const dateFormat = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'Asia/Jakarta' });

type Props = {
  start: Date;
  days: number;
  /** 0-based day index; null = live (all milestones up to now). */
  day: number | null;
  onChange: (day: number | null) => void;
};

/** Replays milestone progress one day per step from the points' `stage_at`; no extra request. */
export function TimelinePlayer({ start, days, day, onChange }: Props) {
  const [playing, setPlaying] = useState(false);
  const last = Math.max(0, days - 1);
  const value = day ?? last;
  const latest = useRef({ value, last, onChange });
  useEffect(() => { latest.current = { value, last, onChange }; });

  useEffect(() => {
    if (!playing) return;
    const timer = setInterval(() => {
      const { value: current, last: end, onChange: emit } = latest.current;
      if (current >= end) {
        setPlaying(false);
        return;
      }
      emit(current + 1);
    }, TIMELINE_STEP_MS);
    return () => clearInterval(timer);
  }, [playing]);

  const toggle = () => {
    if (playing) {
      setPlaying(false);
      return;
    }
    // Starting from "live" or from the last day replays from the first day.
    if (day === null || day >= last) onChange(0);
    setPlaying(true);
  };

  const date = dayStart(start, value);

  return <div className="flex flex-wrap items-center gap-3" role="group" aria-label="Timeline milestone">
    <Button type="button" size="icon" variant="outline" className="size-11" aria-label={playing ? 'Jeda timeline' : 'Putar timeline'} onClick={toggle}>
      {playing ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
    </Button>
    <div className="min-w-0 flex-1 space-y-1">
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="font-medium tabular-nums">{`Hari ke-${value + 1} · ${dateFormat.format(date)}`}</span>
        {day !== null
          ? <button type="button" className="inline-flex min-h-11 items-center px-1 text-xs text-primary underline-offset-2 hover:underline" onClick={() => { setPlaying(false); onChange(null); }}>Kembali ke terkini</button>
          : <span className="text-xs text-muted-foreground">Terkini</span>}
      </div>
      <input
        type="range"
        className="w-full accent-primary"
        aria-label="Tanggal timeline"
        min={0}
        max={last}
        step={1}
        value={value}
        onChange={(event) => { setPlaying(false); onChange(Number(event.target.value)); }}
      />
    </div>
  </div>;
}
