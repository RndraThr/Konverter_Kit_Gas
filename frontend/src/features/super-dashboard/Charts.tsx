import { format, parseISO } from 'date-fns';
import { id } from 'date-fns/locale';
import { Bar, BarChart, CartesianGrid, Legend, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { formatNumber, percent, stages, type StageKey } from './stats';
import type { DashboardDay, DashboardStages } from './types';

const axis = { stroke: '#8a9186', fontSize: 12, tickLine: false, axisLine: false } as const;
const grid = <CartesianGrid stroke="#e4e8e1" strokeDasharray="0" vertical={false} />;
const tooltipStyle = { borderRadius: 8, border: '1px solid #dfe4dc', fontSize: 12 } as const;
const dayLabel = (day: string) => format(parseISO(day), 'd MMM', { locale: id });

type StageRow = DashboardStages & { label: string; id: string };

/**
 * One row per zone or kabupaten, one thin bar per stage as a percentage of
 * its quota (so a small and a big kabupaten compare). The tooltip gives the
 * counts.
 */
export function StageChart({ rows, onOpen }: { rows: StageRow[]; onOpen?: (id: string) => void }) {
  const data = rows.map((row) => ({ ...row, ...Object.fromEntries(stages.map((s) => [`${s.key}_pct`, percent(row[s.key], row.quota)])) }));
  return <ResponsiveContainer width="100%" height={Math.max(160, rows.length * 56 + 56)}>
    <BarChart data={data} layout="vertical" margin={{ top: 4, right: 16, bottom: 4, left: 4 }} barGap={2} barCategoryGap={12}>
      <CartesianGrid stroke="#e4e8e1" horizontal={false} />
      <XAxis type="number" domain={[0, 100]} unit="%" {...axis} />
      <YAxis type="category" dataKey="label" width={150} {...axis} tick={{ fill: '#3d4439', fontSize: 12 }} />
      <Tooltip
        cursor={{ fill: '#eef1eb' }}
        contentStyle={tooltipStyle}
        formatter={(_value, name, item) => {
          const stage = stages.find((s) => s.label === name);
          const row = item.payload as StageRow;
          return stage ? [`${formatNumber(row[stage.key])} / ${formatNumber(row.quota)} (${percent(row[stage.key], row.quota)}%)`, name] : [String(_value), name];
        }}
      />
      <Legend iconType="circle" wrapperStyle={{ fontSize: 12 }} />
      {stages.map((s) => <Bar key={s.key} dataKey={`${s.key}_pct`} name={s.label} fill={s.color} barSize={7} radius={[0, 4, 4, 0]}
        cursor={onOpen ? 'pointer' : undefined}
        onClick={onOpen ? (entry: { payload?: StageRow }) => entry.payload && onOpen(entry.payload.id) : undefined} />)}
    </BarChart>
  </ResponsiveContainer>;
}

export function DailyChart({ days }: { days: DashboardDay[] }) {
  return <ResponsiveContainer width="100%" height={240}>
    <BarChart data={days} margin={{ top: 8, right: 8, bottom: 0, left: -8 }}>
      {grid}
      <XAxis dataKey="date" tickFormatter={dayLabel} {...axis} minTickGap={12} />
      <YAxis allowDecimals={false} {...axis} />
      <Tooltip cursor={{ fill: '#eef1eb' }} contentStyle={tooltipStyle} labelFormatter={(day) => dayLabel(String(day))} formatter={(value) => [formatNumber(Number(value)), 'Diserahkan']} />
      <Bar dataKey="completed" name="Diserahkan" fill={stageColor('completed')} radius={[4, 4, 0, 0]} maxBarSize={28} />
    </BarChart>
  </ResponsiveContainer>;
}

export function CumulativeChart({ days, quota }: { days: (DashboardDay & { total: number })[]; quota: number }) {
  return <ResponsiveContainer width="100%" height={240}>
    <LineChart data={days} margin={{ top: 8, right: 8, bottom: 0, left: -8 }}>
      {grid}
      <XAxis dataKey="date" tickFormatter={dayLabel} {...axis} minTickGap={12} />
      <YAxis allowDecimals={false} domain={[0, Math.max(quota, ...days.map((d) => d.total))]} {...axis} />
      <Tooltip contentStyle={tooltipStyle} labelFormatter={(day) => dayLabel(String(day))} formatter={(value) => [`${formatNumber(Number(value))} (${percent(Number(value), quota)}% kuota)`, 'Total diserahkan']} />
      {quota > 0 ? <ReferenceLine y={quota} stroke="#8a9186" strokeDasharray="4 4" label={{ value: `Kuota ${formatNumber(quota)}`, position: 'insideTopRight', fill: '#5d6559', fontSize: 12 }} /> : null}
      <Line type="monotone" dataKey="total" name="Total diserahkan" stroke={stageColor('completed')} strokeWidth={2} dot={{ r: 4 }} activeDot={{ r: 5 }} />
    </LineChart>
  </ResponsiveContainer>;
}

export function ActivityChart({ rows }: { rows: { label: string; media: number }[] }) {
  return <ResponsiveContainer width="100%" height={Math.max(120, rows.length * 32 + 32)}>
    <BarChart data={rows} layout="vertical" margin={{ top: 4, right: 32, bottom: 4, left: 4 }}>
      <CartesianGrid stroke="#e4e8e1" horizontal={false} />
      <XAxis type="number" allowDecimals={false} {...axis} />
      <YAxis type="category" dataKey="label" width={170} {...axis} tick={{ fill: '#3d4439', fontSize: 12 }} />
      <Tooltip cursor={{ fill: '#eef1eb' }} contentStyle={tooltipStyle} formatter={(value) => [formatNumber(Number(value)), 'Foto & video']} />
      <Bar dataKey="media" name="Foto & video" fill="#2f7d4a" barSize={14} radius={[0, 4, 4, 0]} label={{ position: 'right', fill: '#3d4439', fontSize: 12 }} />
    </BarChart>
  </ResponsiveContainer>;
}

function stageColor(key: StageKey) {
  return stages.find((s) => s.key === key)!.color;
}
