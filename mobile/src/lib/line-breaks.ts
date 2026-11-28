// Scripts written without spaces between words: any line break there falls
// between letters, so it can't be told apart from a word split in two.
const NO_SPACES = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Thai}\p{Script=Lao}\p{Script=Khmer}\p{Script=Myanmar}]/u;
const LETTER = /[\p{L}\p{N}]/u;

/**
 * Whether laid-out text (its lines, as onTextLayout gives them) breaks a word
 * across two lines: a letter on both sides of a break, with no space between.
 * False for scripts that don't separate words with spaces.
 */
export function breaksAWord(lines: string[]): boolean {
  if (lines.some((l) => NO_SPACES.test(l))) return false;
  return lines.some((l, i) => i < lines.length - 1 && LETTER.test(l.slice(-1)) && LETTER.test(lines[i + 1].slice(0, 1)));
}
