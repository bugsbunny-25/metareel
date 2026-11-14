import { HStack, Image, Link, ProgressView, Spacer, Text, VStack } from '@expo/ui/swift-ui';
import { activityBackgroundTint, font, foregroundStyle, labelsHidden, lineLimit, padding, tint } from '@expo/ui/swift-ui/modifiers';
import { createLiveActivity, type LiveActivityEnvironment } from 'expo-widgets';

import type { NowShowingProps } from './props';

// iOS "Now showing": a Live Activity on the lock screen and in the Dynamic
// Island while a film plays. The progress bar and the island's countdown
// count by themselves (SwiftUI timer intervals), so nothing has to update
// it; the lock screen shows start and end times (it hides a countdown's
// seconds). "Log it" opens the log sheet pre-filled.

const NowShowing = (p: NowShowingProps, env: LiveActivityEnvironment) => {
  'widget';
  const accent = env.isLuminanceReduced ? '#ffffff' : '#ff8a4c';
  const range = { lower: new Date(p.startMs), upper: new Date(p.endMs) };
  return {
    banner: (
      <VStack alignment="leading" spacing={8} modifiers={[padding({ all: 14 }), activityBackgroundTint('#0b0b0c')]}>
        <HStack spacing={8}>
          <Image systemName="film" color={accent} />
          <VStack alignment="leading" spacing={1}>
            <Text modifiers={[font({ size: 16, weight: 'bold', design: 'serif' }), foregroundStyle('#f5f5f7'), lineLimit(1)]}>{p.title}</Text>
            {p.venue ? <Text modifiers={[font({ size: 12 }), foregroundStyle('#8e8e93'), lineLimit(1)]}>{p.venue}</Text> : null}
          </VStack>
          <Spacer />
          <Link destination={p.logUrl} label="Log it" modifiers={[font({ size: 14, weight: 'semibold' }), tint(accent)]} />
        </HStack>
        <ProgressView timerInterval={range} countsDown={false} modifiers={[tint(accent), labelsHidden()]} />
        <HStack>
          <Text modifiers={[font({ size: 12 }), foregroundStyle('#8e8e93')]}>{p.startLabel}</Text>
          <Spacer />
          <Text modifiers={[font({ size: 12 }), foregroundStyle('#8e8e93')]}>{p.endLabel}</Text>
        </HStack>
      </VStack>
    ),
    compactLeading: <Image systemName="film" color={accent} />,
    compactTrailing: <Text timerInterval={range} countsDown modifiers={[font({ size: 13 }), foregroundStyle(accent)]} />,
    minimal: <Image systemName="film" color={accent} />,
    expandedLeading: <Image systemName="film" color={accent} />,
    expandedTrailing: <Link destination={p.logUrl} label="Log it" modifiers={[tint(accent)]} />,
    expandedBottom: (
      <VStack alignment="leading" spacing={6}>
        <Text modifiers={[font({ size: 15, weight: 'bold', design: 'serif' }), lineLimit(1)]}>{p.title}</Text>
        <ProgressView timerInterval={range} countsDown={false} modifiers={[tint(accent), labelsHidden()]} />
      </VStack>
    ),
  };
};

export default createLiveActivity<NowShowingProps>('NowShowing', NowShowing);
