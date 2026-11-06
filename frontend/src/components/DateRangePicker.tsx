import { format, parseISO } from 'date-fns';
import { id } from 'date-fns/locale';
import { CalendarDays } from 'lucide-react';
import { useState } from 'react';
import type { DateRange } from 'react-day-picker';
import { Button } from '@/components/ui/button';
import { Calendar } from '@/components/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';

const label = (value: string) => format(parseISO(value), 'd MMM yyyy', { locale: id });
const iso = (value: Date) => format(value, 'yyyy-MM-dd');

type DateRangePickerProps = { from: string; to: string; max?: Date; 'aria-label'?: string; onChange: (from: string, to: string) => void };

/** Rentang tanggal yyyy-MM-dd: klik awal lalu akhir. Tertutup hanya oleh klik di luar. */
export function DateRangePicker({ from, to, max, onChange, ...trigger }: DateRangePickerProps) {
  const [open, setOpen] = useState(false);
  const [range, setRange] = useState<DateRange | undefined>({ from: parseISO(from), to: parseISO(to) });
  const [picking, setPicking] = useState(false);
  return <Popover open={open} onOpenChange={setOpen}>
    <PopoverTrigger render={<Button variant="outline" {...trigger} />}>
      <CalendarDays aria-hidden="true" />
      {from === to ? label(from) : `${label(from)} – ${label(to)}`}
    </PopoverTrigger>
    <PopoverContent align="start" className="w-auto p-0">
      <Calendar
        mode="range"
        locale={id}
        numberOfMonths={2}
        defaultMonth={parseISO(from)}
        disabled={max ? { after: max } : undefined}
        selected={range}
        onSelect={(next, day) => {
          if (!picking) {
            setRange({ from: day, to: undefined });
            setPicking(true);
            return;
          }
          const start = next?.from ?? day;
          const end = next?.to ?? start;
          setRange({ from: start, to: end });
          setPicking(false);
          onChange(iso(start), iso(end));
        }}
      />
    </PopoverContent>
  </Popover>;
}
