import { Image } from 'expo-image';
import { SvgCss } from 'react-native-svg/css';

import { useTheme } from '@/theme';

import { imdbBlack, imdbGold, tmdb } from './rating-logos';

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
 * tipped; TMDB's wordmark. rotten = score under 60.
 */
export function RatingLogo({ source, rotten, certified, height = 18 }: { source: RatingSource; rotten?: boolean; certified?: boolean; height?: number }) {
  const { scheme } = useTheme();
  if (source === 'imdb') {
    return <SvgCss xml={scheme === 'dark' ? imdbGold : imdbBlack} height={height} width={Math.round(height * 2.41)} accessible={false} />;
  }
  if (source === 'tmdb') {
    return <SvgCss xml={tmdb} height={height} width={Math.round(height * 2.33)} accessible={false} />;
  }
  const src = source === 'rt' ? (certified ? RT.certified : rotten ? RT.rotten : RT.fresh) : rotten ? RT.stale : RT.hot;
  return <Image source={src} style={{ width: height, height }} contentFit="contain" accessible={false} />;
}
