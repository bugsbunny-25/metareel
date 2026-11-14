import Svg, { Path } from 'react-native-svg';

// The web app's icon set (app/src/components/Icon.svelte, 20x20 strokes),
// plus the tab icons.
const PATHS = {
  bookmark: 'M6 3.5h8v13l-4-3-4 3z',
  'bookmark-fill': 'M6 3.5h8v13l-4-3-4 3z',
  check: 'M4.5 10.5l3.5 3.5 7.5-8',
  plus: 'M10 4.5v11M4.5 10h11',
  pin: 'M10 17s-5-4.6-5-8.5a5 5 0 0110 0C15 12.4 10 17 10 17z',
  search: 'M8.5 14a5.5 5.5 0 100-11 5.5 5.5 0 000 11zM12.5 12.5l4 4',
  more: 'M5 10h.01M10 10h.01M15 10h.01',
  list: 'M7 5h10M7 10h10M7 15h10M3.5 5h.01M3.5 10h.01M3.5 15h.01',
  // tabs
  tonight: 'M15.5 12.5A6 6 0 017.5 4.5a6 6 0 108 8z',
  diary: 'M5 3.5h9.5v13H5a1.5 1.5 0 01-1.5-1.5V5A1.5 1.5 0 015 3.5zM7 7h5M7 10h5',
  'up-next': 'M7 5.5v9l7-4.5z',
  settings:
    'M10 12.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM10 2.5v2M10 15.5v2M2.5 10h2M15.5 10h2M4.7 4.7l1.4 1.4M13.9 13.9l1.4 1.4M4.7 15.3l1.4-1.4M13.9 6.1l1.4-1.4',
} as const;

export type IconName = keyof typeof PATHS;

export function Icon({ name, size = 20, color }: { name: IconName; size?: number; color: string }) {
  const filled = name.endsWith('-fill');
  return (
    <Svg width={size} height={size} viewBox="0 0 20 20" accessible={false}>
      <Path
        d={PATHS[name]}
        fill={filled ? color : 'none'}
        stroke={color}
        strokeWidth={name === 'more' ? 3 : 1.6}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </Svg>
  );
}
