import { useState, useEffect, useRef, useCallback } from 'react';

export type EqualizerPresetKey =
  | 'flat'
  | 'bass_boost'
  | 'vocal_presence'
  | 'electronic_club'
  | 'acoustic_warm'
  | 'custom';

export interface EqualizerBandConfig {
  frequency: number;
  label: string;
  type: BiquadFilterType;
  q?: number;
}

export const EQUALIZER_BANDS: EqualizerBandConfig[] = [
  { frequency: 60, label: '60 Hz', type: 'lowshelf' },
  { frequency: 250, label: '250 Hz', type: 'peaking', q: 1.0 },
  { frequency: 1000, label: '1 kHz', type: 'peaking', q: 1.0 },
  { frequency: 4000, label: '4 kHz', type: 'peaking', q: 1.0 },
  { frequency: 12000, label: '12 kHz', type: 'highshelf' },
];

export const EQUALIZER_PRESETS: Record<Exclude<EqualizerPresetKey, 'custom'>, number[]> = {
  flat: [0, 0, 0, 0, 0],
  bass_boost: [6, 4, 0, 0, 0],
  vocal_presence: [-2, 0, 4, 3, 1],
  electronic_club: [5, 2, -1, 3, 5],
  acoustic_warm: [3, 2, 0, 1, 3],
};

interface WebAudioGraph {
  ctx: AudioContext;
  source: MediaElementAudioSourceNode;
  filters: BiquadFilterNode[];
  analyser: AnalyserNode;
}

const STORAGE_KEY = 'resonamesh_eq_state';
const audioGraphCache = new WeakMap<HTMLAudioElement, WebAudioGraph>();

function loadPersistedState(): { isEnabled: boolean; preset: EqualizerPresetKey; gains: number[] } {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed.gains) && parsed.gains.length === 5) {
        return {
          isEnabled: typeof parsed.isEnabled === 'boolean' ? parsed.isEnabled : true,
          preset: parsed.preset || 'flat',
          gains: parsed.gains,
        };
      }
    }
  } catch {
    // Fall back to flat defaults if local storage read fails
  }
  return {
    isEnabled: true,
    preset: 'flat',
    gains: [0, 0, 0, 0, 0],
  };
}

export function useAudioEqualizer(audioElement: HTMLAudioElement | null) {
  const [persisted] = useState(loadPersistedState);
  const [isEnabled, setIsEnabled] = useState<boolean>(persisted.isEnabled);
  const [preset, setPreset] = useState<EqualizerPresetKey>(persisted.preset);
  const [gains, setGains] = useState<number[]>(persisted.gains);
  const [analyser, setAnalyser] = useState<AnalyserNode | null>(null);

  const graphRef = useRef<WebAudioGraph | null>(null);

  // Initialize or retrieve existing Web Audio routing graph for the HTMLAudioElement
  useEffect(() => {
    if (!audioElement) return;

    let graph = audioGraphCache.get(audioElement);
    if (!graph) {
      try {
        const AudioContextClass = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
        const ctx = new AudioContextClass();
        const source = ctx.createMediaElementSource(audioElement);

        const filters = EQUALIZER_BANDS.map((band) => {
          const filter = ctx.createBiquadFilter();
          filter.type = band.type;
          filter.frequency.value = band.frequency;
          if (band.q !== undefined) {
            filter.Q.value = band.q;
          }
          filter.gain.value = 0;
          return filter;
        });

        const analyserNode = ctx.createAnalyser();
        analyserNode.fftSize = 64;
        analyserNode.smoothingTimeConstant = 0.8;

        // Route: source -> band0 -> band1 -> band2 -> band3 -> band4 -> analyser -> destination
        source.connect(filters[0]);
        for (let i = 0; i < filters.length - 1; i++) {
          filters[i].connect(filters[i + 1]);
        }
        filters[filters.length - 1].connect(analyserNode);
        analyserNode.connect(ctx.destination);

        graph = { ctx, source, filters, analyser: analyserNode };
        audioGraphCache.set(audioElement, graph);
      } catch (err) {
        console.warn('Failed to initialize Web Audio Equalizer graph:', err);
        return;
      }
    }

    graphRef.current = graph;
    setAnalyser(graph.analyser);

    // Resume AudioContext if suspended (browser autoplay policy)
    const onPlayResume = () => {
      if (graph?.ctx.state === 'suspended') {
        graph.ctx.resume().catch(() => {});
      }
    };
    audioElement.addEventListener('play', onPlayResume);
    return () => {
      audioElement.removeEventListener('play', onPlayResume);
    };
  }, [audioElement]);

  // Apply gains to BiquadFilterNodes
  const applyGains = useCallback(
    (currentGains: number[], enabled: boolean) => {
      const graph = graphRef.current;
      if (!graph) return;

      const currentTime = graph.ctx.currentTime;
      graph.filters.forEach((filter, index) => {
        const targetGain = enabled ? currentGains[index] ?? 0 : 0;
        // Smooth transition to prevent audio pops
        filter.gain.setTargetAtTime(targetGain, currentTime, 0.015);
      });
    },
    []
  );

  useEffect(() => {
    applyGains(gains, isEnabled);
  }, [gains, isEnabled, applyGains]);

  // Save changes to localStorage
  useEffect(() => {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({ isEnabled, preset, gains })
      );
    } catch {
      // Ignore storage write issues
    }
  }, [isEnabled, preset, gains]);

  const setBandGain = useCallback((bandIndex: number, gainValue: number) => {
    // Clamp gain between -12 dB and +12 dB
    const clamped = Math.max(-12, Math.min(12, Math.round(gainValue * 10) / 10));
    setGains((prev) => {
      const updated = [...prev];
      updated[bandIndex] = clamped;
      return updated;
    });
    setPreset('custom');
  }, []);

  const selectPreset = useCallback((presetKey: Exclude<EqualizerPresetKey, 'custom'>) => {
    const targetGains = EQUALIZER_PRESETS[presetKey];
    if (targetGains) {
      setGains([...targetGains]);
      setPreset(presetKey);
    }
  }, []);

  const toggleBypass = useCallback(() => {
    setIsEnabled((prev) => !prev);
  }, []);

  const resetFlat = useCallback(() => {
    selectPreset('flat');
  }, [selectPreset]);

  return {
    isEnabled,
    preset,
    gains,
    analyser,
    setBandGain,
    selectPreset,
    toggleBypass,
    resetFlat,
    bands: EQUALIZER_BANDS,
  };
}
