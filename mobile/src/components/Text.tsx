import { Text as RNText, type TextProps } from 'react-native';

import { tabularNums, useTheme } from '@/theme';

type Variant = 'display' | 'title' | 'heading' | 'body' | 'label' | 'muted' | 'caption';

/** Text in the Screening Room type: Source Serif for names, Source Sans for data. */
export function Text({ variant = 'body', num, style, ...rest }: TextProps & { variant?: Variant; num?: boolean }) {
  const { colors, fonts } = useTheme();
  const v = {
    display: { fontFamily: fonts.display, fontSize: 30, lineHeight: 36, color: colors.textPrimary },
    title: { fontFamily: fonts.display, fontSize: 22, lineHeight: 28, color: colors.textPrimary },
    heading: { fontFamily: fonts.display, fontSize: 18, lineHeight: 23, color: colors.textPrimary },
    body: { fontFamily: fonts.sans, fontSize: 15, lineHeight: 21, color: colors.textPrimary },
    label: { fontFamily: fonts.sansSemi, fontSize: 14, lineHeight: 19, color: colors.textPrimary },
    muted: { fontFamily: fonts.sans, fontSize: 14, lineHeight: 19, color: colors.textMuted },
    caption: { fontFamily: fonts.sansSemi, fontSize: 12.5, lineHeight: 16, color: colors.textMuted },
  }[variant];
  return <RNText style={[v, num && { fontVariant: tabularNums }, style]} {...rest} />;
}
