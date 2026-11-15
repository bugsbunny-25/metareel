import Svg, { Rect, Text as SvgText } from 'react-native-svg';

/**
 * The Top 10 badge, Netflix style: a red rounded square with TOP over 10,
 * drawn so it stays crisp at any size (the web's Top10Badge).
 */
export function Top10Badge({ size = 26 }: { size?: number }) {
  return (
    <Svg width={size} height={size} viewBox="0 0 40 40" accessible={false}>
      <Rect width={40} height={40} rx={6} fill="#e50914" />
      <SvgText x={20} y={16.5} textAnchor="middle" fontWeight="900" fontSize={11.5} fill="#fff">
        TOP
      </SvgText>
      <SvgText x={20} y={35} textAnchor="middle" fontWeight="900" fontSize={19} letterSpacing={-1} fill="#fff">
        10
      </SvgText>
    </Svg>
  );
}
