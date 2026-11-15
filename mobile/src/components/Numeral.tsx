import Svg, { Text as SvgText } from 'react-native-svg';

/**
 * The chart numeral (Tonight's rail, the Charts list): a heavy outlined digit
 * filled with the canvas, clipped to its 0.78em line like the web's .big.
 * overlap pulls the next thing (a poster) over its right edge.
 */
export function Numeral({
  value,
  size,
  fill,
  stroke,
  font,
  overlap = 0,
  strokeWidth = 2,
}: {
  value: number;
  size: number;
  fill: string;
  stroke: string;
  font: string;
  overlap?: number;
  strokeWidth?: number;
}) {
  const digits = String(value).length;
  const width = Math.ceil(size * (0.52 * digits - 0.06 * (digits - 1))) + 4;
  const height = Math.round(size * 0.78) + 2;
  return (
    <Svg width={width} height={height} style={overlap ? { marginRight: -overlap } : undefined} accessible={false}>
      <SvgText
        x={width - 2}
        y={height - 3}
        textAnchor="end"
        fontFamily={font}
        fontSize={size}
        letterSpacing={-0.06 * size}
        fill={fill}
        stroke={stroke}
        strokeWidth={strokeWidth}
      >
        {String(value)}
      </SvgText>
    </Svg>
  );
}
