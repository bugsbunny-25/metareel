import * as BackgroundTask from 'expo-background-task';
import * as TaskManager from 'expo-task-manager';

import { currentSession, restoreSession } from '@/auth/session';

import { flushOutbox } from './index';

// Sends queued writes when the OS gives the app background time (every
// 30 minutes at best; the OS decides). Defined at module scope, imported from
// the entry file, so it exists when the app is started headless.
const TASK = 'metareel-outbox';

TaskManager.defineTask(TASK, async () => {
  try {
    if (!currentSession()) await restoreSession();
    await flushOutbox();
    return BackgroundTask.BackgroundTaskResult.Success;
  } catch {
    return BackgroundTask.BackgroundTaskResult.Failed;
  }
});

export async function registerBackgroundSync() {
  try {
    if (!(await TaskManager.isTaskRegisteredAsync(TASK))) {
      await BackgroundTask.registerTaskAsync(TASK, { minimumInterval: 30 });
    }
  } catch {
    // Background work unavailable (e.g. restricted by the OS): the app still
    // flushes on launch, on foreground and when the network comes back.
  }
}
