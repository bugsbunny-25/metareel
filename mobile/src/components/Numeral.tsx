import { useId } from 'react';
import Svg, { Defs, G, LinearGradient, Rect, Stop, Text as SvgText } from 'react-native-svg';

import { numeralLayout, TRACKING } from '@/lib/numeral';
import { useTheme } from '@/theme';

/**
 * The chart numeral (Tonight's rail, the Charts list), like the web's
 * Numeral.svelte: Barlow Condensed ExtraBold filled with the canvas, its
 * outline a gradient lit from the top (orange for your titles), a lone 1
 * standing on a foot (lib/numeral.ts has the geometry). size is the font
 * size; strokeWidth the outline in points. The outline is drawn at twice the
 * width under the fill, so only its outer half shows and the foot joins the
 * 1's stem as one shape. overlap pulls the next thing (a poster) over it.
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
  const L = numeralLayout(value);
  const k = size / 100; // points per layout unit
  const [top, mid, bottom] = mine
    ? [colors.numeralYoursTop, colors.numeralYoursMid, colors.numeralYoursBottom]
    : [colors.numeralTop, colors.numeralMid, colors.numeralBottom];
  const layer = (edge: boolean) => (
    <G key={edge ? 'edge' : 'body'} fill={colors.bgCanvas} stroke={edge ? `url(#${id})` : 'none'} strokeWidth={(strokeWidth * 2) / k} strokeLinejoin="round">
      <SvgText x={L.x} y={L.baseline} fontFamily={fonts.numeral} fontSize={100} letterSpacing={TRACKING}>
        {String(value)}
      </SvgText>
      {L.foot ? <Rect x={L.foot.x} y={L.foot.y} width={L.foot.w} height={L.foot.h} /> : null}
    </G>
  );
  return (
    <Svg
      width={L.width * k}
      height={L.height * k}
      viewBox={`0 0 ${L.width} ${L.height}`}
      style={overlap ? { marginRight: -overlap } : undefined}
      accessible={false}
    >
      <Defs>
        <LinearGradient id={id} x1="0" y1="0" x2="0" y2={L.height} gradientUnits="userSpaceOnUse">
          <Stop offset="0" stopColor={top} />
          <Stop offset="0.55" stopColor={mid} />
          <Stop offset="1" stopColor={bottom} />
        </LinearGradient>
      </Defs>
      {layer(true)}
      {layer(false)}
    </Svg>
  );
}
