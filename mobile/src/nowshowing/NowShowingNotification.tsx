import { Column, LinearProgressIndicator, Text } from '@expo/ui/jetpack-compose';
import { fillMaxWidth, paddingAll } from '@expo/ui/jetpack-compose/modifiers';
import { createCustomNotification } from 'expo-widgets';

import type { NowShowingProps } from './props';

// Android "Now showing": an ongoing notification while a film plays. Its
// views are drawn once per post or update (no live timers), so it shows the
// start and expected end and the app refreshes the progress when it comes
// to the foreground. A tap opens the log sheet pre-filled.

export default createCustomNotification<NowShowingProps>('NowShowing', (p, env) => {
  'widget';
  const fg = env.colorScheme === 'dark' ? '#f5f5f7' : '#1d1d1f';
  const muted = env.colorScheme === 'dark' ? '#8e8e93' : '#6e6e73';
  const body = (
    <Column modifiers={[fillMaxWidth(), paddingAll(8)]} verticalArrangement={{ spacedBy: 4 }}>
      <Text color={fg} maxLines={1} overflow="ellipsis" style={{ fontSize: 15, fontWeight: '700' }}>{p.title}</Text>
      <Text color={muted} maxLines={1} overflow="ellipsis" style={{ fontSize: 12 }}>
        {[p.venue, p.startLabel, p.endLabel].filter(Boolean).join(' · ')}
      </Text>
      <LinearProgressIndicator progress={p.progress} color="#ff8a4c" trackColor={muted} modifiers={[fillMaxWidth()]} />
    </Column>
  );
  return { collapsed: body };
});
