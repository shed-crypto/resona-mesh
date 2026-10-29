export interface LyricLine {
  timeSec: number;
  text: string;
}

/**
 * Parses raw LRC string into a sorted array of LyricLine objects.
 * Supports standard [mm:ss.xx], [mm:ss.xxx], [mm:ss] and multi-tag lines.
 * Accurately extracts and applies optional [offset:+/-ms].
 */
export function parseLRC(rawLRC: string): LyricLine[] {
  if (!rawLRC || typeof rawLRC !== 'string') {
    return [];
  }

  const lines = rawLRC.split(/\r?\n/);
  const result: LyricLine[] = [];
  let offsetMs = 0;

  // Pattern for time tags: [01:23.45] or [01:23.456] or [01:23]
  const timeTagRegex = /\[(\d{1,3}):(\d{2})(?:\.(\d{1,3}))?\]/g;
  // Pattern for offset tag: [offset:+/-1000]
  const offsetRegex = /\[offset:\s*([+-]?\d+)\s*\]/i;

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }

    // Check for offset tag
    const offsetMatch = trimmed.match(offsetRegex);
    if (offsetMatch) {
      const parsedOffset = parseInt(offsetMatch[1], 10);
      if (!isNaN(parsedOffset)) {
        offsetMs = parsedOffset;
      }
      continue;
    }

    // Match all timestamp tags in the current line
    timeTagRegex.lastIndex = 0;
    const matches = Array.from(trimmed.matchAll(timeTagRegex));
    if (matches.length === 0) {
      // Line is an ID tag (e.g. [ar:Artist], [ti:Title]) or untimed commentary
      continue;
    }

    // Extract the lyric text after stripping all timestamp tags
    const lyricText = trimmed.replace(timeTagRegex, '').trim();

    for (const match of matches) {
      const minutes = parseInt(match[1], 10);
      const seconds = parseInt(match[2], 10);
      const fractionStr = match[3] || '0';

      // Standardize fraction: "5" -> 500ms, "50" -> 500ms, "500" -> 500ms
      let fractionSec = 0;
      if (fractionStr.length === 1) {
        fractionSec = parseInt(fractionStr, 10) / 10;
      } else if (fractionStr.length === 2) {
        fractionSec = parseInt(fractionStr, 10) / 100;
      } else {
        fractionSec = parseInt(fractionStr.slice(0, 3), 10) / 1000;
      }

      let totalSec = minutes * 60 + seconds + fractionSec;
      if (offsetMs !== 0) {
        totalSec += offsetMs / 1000;
      }

      result.push({
        timeSec: Math.max(0, totalSec),
        text: lyricText,
      });
    }
  }

  // Sort lines chronologically
  result.sort((a, b) => a.timeSec - b.timeSec);
  return result;
}

/**
 * Performs binary search to find the active lyric line index for a given playback time.
 * Returns the highest index where lines[i].timeSec <= currentTimeSec.
 * Returns -1 if currentTimeSec is before the first lyric line or lines is empty.
 */
export function findActiveLyricIndex(lines: LyricLine[], currentTimeSec: number): number {
  if (!lines || lines.length === 0 || currentTimeSec < lines[0].timeSec) {
    return -1;
  }

  let low = 0;
  let high = lines.length - 1;
  let bestIndex = -1;

  while (low <= high) {
    const mid = (low + high) >> 1;
    if (lines[mid].timeSec <= currentTimeSec) {
      bestIndex = mid;
      low = mid + 1; // Try to find a later matching line
    } else {
      high = mid - 1;
    }
  }

  return bestIndex;
}

/**
 * Formats seconds into standard [mm:ss.xx] timestamp for LRC export/authoring.
 */
export function formatLrcTimestamp(seconds: number): string {
  if (isNaN(seconds) || seconds < 0) {
    return '[00:00.00]';
  }
  const mins = Math.floor(seconds / 60);
  const remainingSecs = seconds - mins * 60;
  const wholeSecs = Math.floor(remainingSecs);
  const hundredths = Math.floor((remainingSecs - wholeSecs) * 100);

  const mm = String(mins).padStart(2, '0');
  const ss = String(wholeSecs).padStart(2, '0');
  const xx = String(hundredths).padStart(2, '0');

  return `[${mm}:${ss}.${xx}]`;
}
