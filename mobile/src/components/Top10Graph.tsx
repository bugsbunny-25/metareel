import { useState } from 'react';
import { View } from 'react-native';
import Svg, { Circle, Line, Path, Text as SvgText } from 'react-native-svg';

import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { usePreferences, useTitleRankings } from '@/data/queries';
import { providerName, useChartReference, useScope } from '@/data/scope';
import { useTheme } from '@/theme';

const COLORS = ['#e58a4a', '#8eb4f0', '#30d158', '#bf5af2', '#ffd60a', '#ff6482'];
const H = 170;
const PAD = { l: 26, r: 10, t: 10, b: 24 };

const dayMonth = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });

/**
 * A title's Top 10 run in one country (the web's Top10Graph): rank by day,
 * 1 at the top, one line per service, a gap where it left the chart. The
 * country is the first in your country order (Settings) the title charted
 * in; with no order set, the chart country you browse. Nothing when none.
 */
export function Top10Graph({ kind, id }: { kind: 'movie' | 'tv'; id: string }) {
  const { colors, fonts } = useTheme();
  const prefs = usePreferences();
  const scope = useScope({ country: prefs.data?.country, provider: prefs.data?.provider });
  const q = useTitleRankings(kind, id);
  const ref = useChartReference();
  const [width, setWidth] = useState(0);
  const rankings = q.data?.rankings ?? [];
  const order = [...new Set(prefs.data?.countries?.length ? prefs.data.countries : [scope.country, prefs.data?.country].filter((c): c is string => !!c))];
  const country = order.find((c) => rankings.some((r) => r.country === c));
  if (!country) return null;
  const rows = rankings.filter((r) => r.country === country && r.date && r.rank);
  const days = [...new Set(rows.map((r) => r.date!))].sort();
  const t0 = Date.parse(days[0]);
  const span = Math.max(1, (Date.parse(days[days.length - 1]) - t0) / 86_400_000);
  const W = Math.max(1, width);
  const x = (d: string) => PAD.l + ((Date.parse(d) - t0) / 86_400_000 / span) * (W - PAD.l - PAD.r);
  const y = (rank: number) => PAD.t + ((rank - 1) / 9) * (H - PAD.t - PAD.b);
  const providers = [...new Set(rows.map((r) => r.provider!))];
  const lines = providers.map((p, i) => {
    const pts = rows.filter((r) => r.provider === p).sort((a, b) => (a.date! < b.date! ? -1 : 1));
    let d = '';
    let prev: string | null = null;
    for (const r of pts) {
      const gap = prev && (Date.parse(r.date!) - Date.parse(prev)) / 86_400_000 > 1.5;
      d += `${!prev || gap ? 'M' : 'L'}${x(r.date!).toFixed(1)},${y(r.rank!).toFixed(1)}`;
      prev = r.date!;
    }
    return { provider: p, color: COLORS[i % COLORS.length], d, pts, best: Math.min(...pts.map((r) => r.rank!)) };
  });
  const ticks = [...new Set([days[0], days[Math.floor(days.length / 2)], days[days.length - 1]])];
  // Hermes has no Intl.DisplayNames: names come from the chart reference list.
  const name = ref.data?.countries.find((c) => c.code === country)?.name ?? country;

  return (
    <Panel title={`Top 10 in ${name}`}>
      <View
        onLayout={(e) => setWidth(Math.round(e.nativeEvent.layout.width))}
        accessible
        accessibilityLabel={`Daily Top 10 rank in ${name}: ${lines.map((l) => `${providerName(l.provider)} best number ${l.best} over ${l.pts.length} days`).join('; ')}`}
      >
        {width ? (
          <Svg width={W} height={H}>
            {[1, 5, 10].map((r) => (
              <Line key={r} x1={PAD.l} x2={W - PAD.r} y1={y(r)} y2={y(r)} stroke={colors.borderSubtle} strokeWidth={1} />
            ))}
            {[1, 5, 10].map((r) => (
              <SvgText key={`l${r}`} x={PAD.l - 7} y={y(r) + 4} fontSize={11} fill={colors.textMuted} textAnchor="end" fontFamily={fonts.sans}>
                {String(r)}
              </SvgText>
            ))}
            {ticks.map((d, i) => (
              <SvgText
                key={d}
                x={x(d)}
                y={H - 6}
                fontSize={11}
                fill={colors.textMuted}
                fontFamily={fonts.sans}
                textAnchor={i === 0 ? 'start' : i === ticks.length - 1 ? 'end' : 'middle'}
              >
                {dayMonth(d)}
              </SvgText>
            ))}
            {lines.map((l) => (
              <Path key={l.provider} d={l.d} fill="none" stroke={l.color} strokeWidth={2.5} strokeLinejoin="round" strokeLinecap="round" />
            ))}
            {lines.flatMap((l) => l.pts.map((p) => <Circle key={`${l.provider}${p.date}`} cx={x(p.date!)} cy={y(p.rank!)} r={days.length === 1 ? 5 : 2.6} fill={l.color} />))}
          </Svg>
        ) : (
          <View style={{ height: H }} />
        )}
      </View>
      <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 14 }}>
        {lines.map((l) => (
          <View key={l.provider} style={{ flexDirection: 'row', alignItems: 'center', gap: 6 }}>
            <View style={{ width: 10, height: 10, borderRadius: 5, backgroundColor: l.color }} />
            <Text variant="label">{providerName(l.provider)}</Text>
            <Text variant="caption" num>
              best No. {l.best} · {l.pts.length} {l.pts.length === 1 ? 'day' : 'days'}
            </Text>
          </View>
        ))}
      </View>
    </Panel>
  );
}
