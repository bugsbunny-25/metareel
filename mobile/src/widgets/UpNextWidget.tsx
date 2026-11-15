import { Column, Text as CText } from '@expo/ui/jetpack-compose';
import { background, fillMaxSize, paddingAll } from '@expo/ui/jetpack-compose/modifiers';
import { Spacer, Text, VStack } from '@expo/ui/swift-ui';
import { containerBackground, font, foregroundStyle, lineLimit, widgetURL } from '@expo/ui/swift-ui/modifiers';
import { createWidget, type WidgetEnvironment } from 'expo-widgets';
import { Platform } from 'react-native';

import type { UpNextProps } from './props';

// Up next: the next episode of the shows in progress, and what airs next.
// Home screen (small, medium) and lock screen (rectangular, inline) on iOS;
// a 2x2 home screen widget on Android.

const UpNextIOS = (p: UpNextProps, env: WidgetEnvironment) => {
  'widget';
  const muted = '#8e8e93';
  const bg = [containerBackground('#0b0b0c', 'widget'), widgetURL(p.url)];
  const first = p.items[0];
  if (env.widgetFamily === 'accessoryInline') {
    return <Text>{first ? `${first.title} ${first.line}` : p.airing || 'Nothing up next'}</Text>;
  }
  if (env.widgetFamily === 'accessoryRectangular') {
    return (
      <VStack alignment="leading" spacing={1} modifiers={[widgetURL(p.url)]}>
        <Text modifiers={[font({ size: 13, weight: 'semibold' }), lineLimit(1)]}>{first?.title ?? 'Up next'}</Text>
        <Text modifiers={[font({ size: 12 }), lineLimit(1)]}>{first?.line ?? (p.airing || 'Nothing in progress')}</Text>
      </VStack>
    );
  }
  if (p.state !== 'ok') {
    return (
      <VStack alignment="leading" spacing={4} modifiers={bg}>
        <Text modifiers={[font({ size: 11, weight: 'semibold' }), foregroundStyle(muted)]}>UP NEXT</Text>
        <Spacer />
        <Text modifiers={[font({ size: 14 }), foregroundStyle('#f5f5f7')]}>
          {p.state === 'signedOut' ? 'Sign in to metareel' : 'Tick an episode of a show to start'}
        </Text>
      </VStack>
    );
  }
  const rows = env.widgetFamily === 'systemSmall' ? p.items.slice(0, 1) : p.items;
  return (
    <VStack alignment="leading" spacing={6} modifiers={bg}>
      <Text modifiers={[font({ size: 11, weight: 'semibold' }), foregroundStyle(muted)]}>UP NEXT</Text>
      {rows.map((r) => (
        <VStack key={r.title} alignment="leading" spacing={1}>
          <Text modifiers={[font({ size: 15, weight: 'bold', design: 'serif' }), foregroundStyle('#f5f5f7'), lineLimit(env.widgetFamily === 'systemSmall' ? 2 : 1)]}>
            {r.title}
          </Text>
          <Text modifiers={[font({ size: 12 }), foregroundStyle(muted), lineLimit(1)]}>{r.line}</Text>
        </VStack>
      ))}
      <Spacer />
      {p.airing ? <Text modifiers={[font({ size: 11 }), foregroundStyle('#ff8a4c'), lineLimit(1)]}>{`Next: ${p.airing}`}</Text> : null}
    </VStack>
  );
};

const UpNextAndroid = (p: UpNextProps) => {
  'widget';
  const first = p.items[0];
  return (
    <Column modifiers={[fillMaxSize(), background('#0b0b0c'), paddingAll(14)]} verticalArrangement={{ spacedBy: 4 }}>
      <CText color="#8e8e93" style={{ fontSize: 11, fontWeight: '600' }}>UP NEXT</CText>
      {p.state !== 'ok' || !first ? (
        <CText color="#f5f5f7" style={{ fontSize: 14 }}>
          {p.state === 'signedOut' ? 'Sign in to metareel' : p.airing ? `Next: ${p.airing}` : 'Tick an episode of a show to start'}
        </CText>
      ) : (
        <Column verticalArrangement={{ spacedBy: 2 }}>
          <CText color="#f5f5f7" maxLines={2} overflow="ellipsis" style={{ fontSize: 16, fontWeight: '700', fontFamily: 'serif' }}>
            {first.title}
          </CText>
          <CText color="#8e8e93" maxLines={1} overflow="ellipsis" style={{ fontSize: 12 }}>{first.line}</CText>
          {p.airing ? (
            <CText color="#ff8a4c" maxLines={1} overflow="ellipsis" style={{ fontSize: 11 }}>{`Next: ${p.airing}`}</CText>
          ) : null}
        </Column>
      )}
    </Column>
  );
};

export default createWidget<UpNextProps>('UpNext', Platform.OS === 'ios' ? UpNextIOS : UpNextAndroid, {
  state: 'signedOut',
  items: [],
  airing: '',
  url: 'metareel:///',
});
