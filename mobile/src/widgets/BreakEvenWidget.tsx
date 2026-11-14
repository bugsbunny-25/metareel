import { Column, LinearProgressIndicator, Text as CText } from '@expo/ui/jetpack-compose';
import { background, fillMaxSize, fillMaxWidth, paddingAll } from '@expo/ui/jetpack-compose/modifiers';
import { ProgressView, Spacer, Text, VStack } from '@expo/ui/swift-ui';
import { containerBackground, font, foregroundStyle, lineLimit, tint, widgetURL } from '@expo/ui/swift-ui/modifiers';
import { createWidget } from 'expo-widgets';
import { Platform } from 'react-native';

import type { BreakEvenProps } from './props';

// Break-even: the cinema plan's month against the films it takes to pay off.

const BreakEvenIOS = (p: BreakEvenProps) => {
  'widget';
  const muted = '#8e8e93';
  return (
    <VStack alignment="leading" spacing={6} modifiers={[containerBackground('#0b0b0c', 'widget'), widgetURL(p.url)]}>
      <Text modifiers={[font({ size: 11, weight: 'semibold' }), foregroundStyle(muted), lineLimit(1)]}>
        {p.state === 'ok' ? p.plan.toUpperCase() : 'CINEMA PLAN'}
      </Text>
      {p.state === 'ok' ? (
        <>
          <Text modifiers={[font({ size: 15, weight: 'bold', design: 'serif' }), foregroundStyle('#f5f5f7'), lineLimit(2)]}>{p.progressLabel}</Text>
          <ProgressView value={p.progress} modifiers={[tint('#ff8a4c')]} />
          <Spacer />
          <Text modifiers={[font({ size: 11 }), foregroundStyle(muted), lineLimit(2)]}>{p.balance}</Text>
        </>
      ) : (
        <Text modifiers={[font({ size: 14 }), foregroundStyle('#f5f5f7')]}>
          {p.state === 'signedOut' ? 'Sign in to metareel' : 'Add a cinema subscription to track its break-even'}
        </Text>
      )}
    </VStack>
  );
};

const BreakEvenAndroid = (p: BreakEvenProps) => {
  'widget';
  return (
    <Column modifiers={[fillMaxSize(), background('#0b0b0c'), paddingAll(14)]} verticalArrangement={{ spacedBy: 6 }}>
      <CText color="#8e8e93" maxLines={1} style={{ fontSize: 11, fontWeight: '600' }}>
        {p.state === 'ok' ? p.plan.toUpperCase() : 'CINEMA PLAN'}
      </CText>
      {p.state === 'ok' ? (
        <Column verticalArrangement={{ spacedBy: 6 }}>
          <CText color="#f5f5f7" maxLines={2} style={{ fontSize: 15, fontWeight: '700', fontFamily: 'serif' }}>{p.progressLabel}</CText>
          <LinearProgressIndicator progress={p.progress} color="#ff8a4c" trackColor="#2c2c2e" modifiers={[fillMaxWidth()]} />
          <CText color="#8e8e93" maxLines={2} style={{ fontSize: 11 }}>{p.balance}</CText>
        </Column>
      ) : (
        <CText color="#f5f5f7" style={{ fontSize: 14 }}>
          {p.state === 'signedOut' ? 'Sign in to metareel' : 'Add a cinema subscription to track its break-even'}
        </CText>
      )}
    </Column>
  );
};

export default createWidget<BreakEvenProps>('BreakEven', Platform.OS === 'ios' ? BreakEvenIOS : BreakEvenAndroid, {
  state: 'signedOut',
  plan: '',
  progressLabel: '',
  progress: 0,
  balance: '',
  url: 'metareel:///',
});
