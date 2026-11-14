import { Panel, Screen } from '@/components/Screen';
import { Text } from '@/components/Text';

export default function Diary() {
  return (
    <Screen title="Diary">
      <Panel>
        <Text variant="muted">Your watches, newest first, with edit and delete. Coming in the next build.</Text>
      </Panel>
    </Screen>
  );
}
