import { useId } from 'react';
import Svg, { Defs, LinearGradient, Stop, Text as SvgText } from 'react-native-svg';

import { useTheme } from '@/theme';

/**
 * The chart numeral (Tonight's rail, the Charts list): a heavy digit filled
 * with the canvas, its outline a gradient lit from the top (orange for your
 * titles), clipped to its 0.78em line like the web's Numeral.svelte.
 * overlap pulls the next thing (a poster) over its right edge.
 */
export function Numeral({
  value,
  size,
  mine = false,
  overlap = 0,
  strokeWidth = 2,
}: {
  value: number;
  size: number;
  mine?: boolean;
  overlap?: number;
  strokeWidth?: number;
}) {
  const { colors, fonts } = useTheme();
  const id = `numeral${useId().replace(/[^a-zA-Z0-9]/g, '')}`;
  const digits = String(value).length;
  const width = Math.ceil(size * (0.52 * digits - 0.06 * (digits - 1))) + 4;
  const height = Math.round(size * 0.78) + 2;
  const [top, mid, bottom] = mine
    ? [colors.numeralYoursTop, colors.numeralYoursMid, colors.numeralYoursBottom]
    : [colors.numeralTop, colors.numeralMid, colors.numeralBottom];
  return (
    <Svg width={width} height={height} style={overlap ? { marginRight: -overlap } : undefined} accessible={false}>
      <Defs>
        <LinearGradient id={id} x1="0" y1="0" x2="0" y2={height} gradientUnits="userSpaceOnUse">
          <Stop offset="0" stopColor={top} />
          <Stop offset="0.55" stopColor={mid} />
          <Stop offset="1" stopColor={bottom} />
        </LinearGradient>
      </Defs>
      <SvgText
        x={width - 2}
        y={height - 3}
        textAnchor="end"
        fontFamily={fonts.numeral}
        fontSize={size}
        letterSpacing={-0.06 * size}
        fill={colors.bgCanvas}
        stroke={`url(#${id})`}
        strokeWidth={strokeWidth}
      >
        {String(value)}
      </SvgText>
    </Svg>
  );
}
