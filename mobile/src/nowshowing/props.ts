/** What "Now showing" displays (the Live Activity and the Android notification). */
export type NowShowingProps = {
  title: string;
  venue: string;
  startMs: number;
  /** Start plus the runtime (two hours when it isn't known). */
  endMs: number;
  /** e.g. "Started 7:30 PM". */
  startLabel: string;
  /** e.g. "Ends about 9:45 PM". */
  endLabel: string;
  /** 0..1 when posted (the Android notification can't count by itself). */
  progress: number;
  logUrl: string;
};
