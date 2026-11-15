import { endpoint, useSignedIn } from '@/auth/session';
import { useEndpointState } from '@/net/endpoint';

import { Text } from './Text';

/** Which address the app is talking to right now. */
export function ConnectionNote() {
  const session = useSignedIn();
  const ep = useEndpointState(endpoint);
  const via = ep.route === 'home' ? 'home network' : 'server';
  return (
    <Text variant="caption">
      {session.user.username} · via {via}
    </Text>
  );
}
