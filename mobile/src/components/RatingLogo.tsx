import { Image } from 'expo-image';
import Svg, { Defs, LinearGradient, Path, Stop } from 'react-native-svg';

import { useTheme } from '@/theme';

import { IMDB_BLACK, IMDB_GOLD, IMDB_PATH, IMDB_VIEWBOX } from './rating-logos';

const RT = {
  fresh: require('../../assets/ratings/rt-fresh.png'),
  rotten: require('../../assets/ratings/rt-rotten.png'),
  certified: require('../../assets/ratings/rt-certified-fresh.png'),
  hot: require('../../assets/ratings/rt-popcorn-hot.png'),
  stale: require('../../assets/ratings/rt-popcorn-stale.png'),
};

export type RatingSource = 'imdb' | 'rt' | 'rt-audience' | 'tmdb';

/**
 * A ratings source's logo in place of its name (the web app's RatingLogo):
 * IMDb gold on dark and black on light; the Tomatometer's fresh tomato,
 * rotten splat or Certified Fresh badge; the audience popcorn, full or
 * tipped; a star for TMDB. rotten = score under 60.
 */
export function RatingLogo({ source, rotten, certified, height = 18 }: { source: RatingSource; rotten?: boolean; certified?: boolean; height?: number }) {
  const { scheme } = useTheme();
  if (source === 'imdb') {
    return (
      <Svg width={Math.round(height * 2.41)} height={height} viewBox={IMDB_VIEWBOX} accessible={false}>
        <Path fill={scheme === 'dark' ? IMDB_GOLD : IMDB_BLACK} d={IMDB_PATH} />
      </Svg>
    );
  }
  if (source === 'tmdb') {
    // TMDB's score: a star in TMDB's green-to-blue, drawn (an emoji differs on every system).
    return (
      <Svg width={height} height={height} viewBox="0 0 24 24" accessible={false}>
        <Defs>
          <LinearGradient id="tmdbStar" x1="0" y1="0" x2="1" y2="1">
            <Stop offset="0" stopColor="#90cea1" />
            <Stop offset="0.56" stopColor="#3cbec9" />
            <Stop offset="1" stopColor="#00b3e5" />
          </LinearGradient>
        </Defs>
        <Path fill="url(#tmdbStar)" d="M12 1.8l3.1 6.6 7.2.9-5.3 5 1.4 7.1L12 17.9l-6.4 3.5L7 14.3l-5.3-5 7.2-.9z" />
      </Svg>
    );
  }
  const src = source === 'rt' ? (certified ? RT.certified : rotten ? RT.rotten : RT.fresh) : rotten ? RT.stale : RT.hot;
  return <Image source={src} style={{ width: height, height }} contentFit="contain" accessible={false} />;
}
