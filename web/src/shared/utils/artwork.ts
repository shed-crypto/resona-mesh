/**
 * Generates deterministic procedural neon vinyl / audio waveform artwork
 * derived from a track's SHA-256 content_id.
 */

const PALETTES = [
  { primary: '#10b981', secondary: '#064e3b', accent: '#34d399', name: 'Emerald Echo' },
  { primary: '#06b6d4', secondary: '#164e63', accent: '#67e8f9', name: 'Cyan Pulse' },
  { primary: '#8b5cf6', secondary: '#4c1d95', accent: '#c4b5fd', name: 'Violet Haze' },
  { primary: '#f59e0b', secondary: '#78350f', accent: '#fcd34d', name: 'Amber Glow' },
  { primary: '#ec4899', secondary: '#831843', accent: '#f472b6', name: 'Neon Rose' },
  { primary: '#3b82f6', secondary: '#1e3a8a', accent: '#93c5fd', name: 'Deep Cobalt' },
];

function hashString(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = (hash << 5) - hash + str.charCodeAt(i);
    hash |= 0;
  }
  return Math.abs(hash);
}

export function generateCoverDataUri(contentId: string, title: string = ''): string {
  const hash = hashString(contentId || title || 'resonamesh');
  const palette = PALETTES[hash % PALETTES.length];
  const angle = (hash % 360);

  // Procedural concentric grooves and center vinyl label
  const svg = `
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="100%" height="100%">
  <defs>
    <radialGradient id="vinyl-base" cx="50%" cy="50%" r="50%">
      <stop offset="0%" stop-color="#181c24" />
      <stop offset="70%" stop-color="#0e1117" />
      <stop offset="100%" stop-color="#07090c" />
    </radialGradient>
    <linearGradient id="sheen" x1="0%" y1="0%" x2="100%" y2="100%" gradientTransform="rotate(${angle})">
      <stop offset="0%" stop-color="rgba(255,255,255,0.08)" />
      <stop offset="50%" stop-color="rgba(255,255,255,0.0)" />
      <stop offset="100%" stop-color="rgba(255,255,255,0.06)" />
    </linearGradient>
    <radialGradient id="center-label" cx="50%" cy="50%" r="50%">
      <stop offset="0%" stop-color="${palette.primary}" />
      <stop offset="70%" stop-color="${palette.secondary}" />
      <stop offset="100%" stop-color="#0a0d12" />
    </radialGradient>
  </defs>

  <!-- Vinyl Outer Disc -->
  <rect width="200" height="200" fill="#0b0d11" rx="12" />
  <circle cx="100" cy="100" r="92" fill="url(#vinyl-base)" stroke="#222734" stroke-width="1.5" />

  <!-- Concentric Sound Grooves -->
  <circle cx="100" cy="100" r="84" fill="none" stroke="rgba(255,255,255,0.04)" stroke-width="1" />
  <circle cx="100" cy="100" r="76" fill="none" stroke="rgba(255,255,255,0.03)" stroke-width="0.8" />
  <circle cx="100" cy="100" r="68" fill="none" stroke="rgba(255,255,255,0.04)" stroke-width="0.8" />
  <circle cx="100" cy="100" r="60" fill="none" stroke="rgba(255,255,255,0.03)" stroke-width="0.8" />
  <circle cx="100" cy="100" r="52" fill="none" stroke="rgba(255,255,255,0.04)" stroke-width="0.8" />

  <!-- Vinyl Light Sheen (Reflection) -->
  <circle cx="100" cy="100" r="92" fill="url(#sheen)" />

  <!-- Center Label -->
  <circle cx="100" cy="100" r="38" fill="url(#center-label)" stroke="${palette.accent}" stroke-width="1.5" />

  <!-- Geometric Soundwave Arcs -->
  <path d="M 75 100 A 25 25 0 0 1 125 100" fill="none" stroke="${palette.accent}" stroke-width="2.5" stroke-linecap="round" stroke-dasharray="4, 3" />
  <path d="M 82 100 A 18 18 0 0 0 118 100" fill="none" stroke="#ffffff" stroke-width="1.8" stroke-linecap="round" opacity="0.8" />

  <!-- Center Spindle Hole -->
  <circle cx="100" cy="100" r="7" fill="#07090c" stroke="rgba(255,255,255,0.3)" stroke-width="1" />
</svg>
`.trim();

  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}
