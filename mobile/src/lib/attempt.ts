/** The message to show for something thrown. */
export function errorMessage(err: unknown, fallback?: string): string {
  return err instanceof Error ? err.message : (fallback ?? String(err));
}

/**
 * Runs `work` and resolves to the message it failed with, or null when it
 * succeeded. Components call this instead of holding a try/catch/finally
 * themselves: React Compiler skips a component whose body has a `finally`, or
 * a conditional inside a `try`, so the try lives here.
 */
export async function attempt(work: () => unknown, fallback?: string): Promise<string | null> {
  try {
    await work();
    return null;
  } catch (err) {
    return errorMessage(err, fallback);
  }
}
