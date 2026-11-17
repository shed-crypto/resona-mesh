import { useState, useEffect, useRef, useCallback } from 'react';

export type SleepTimerOption = 0 | 15 | 30 | 45 | 60 | -1; // 0 = off, -1 = end of track, positive = minutes

export function useSleepTimer(
  audioElement: HTMLAudioElement | null,
  nominalVolume: number,
  isMuted: boolean,
  onPause: () => void
) {
  const [timerOption, setTimerOption] = useState<SleepTimerOption>(0);
  const [remainingSeconds, setRemainingSeconds] = useState<number | null>(null);

  const nominalVolumeRef = useRef(nominalVolume);
  useEffect(() => {
    nominalVolumeRef.current = nominalVolume;
  }, [nominalVolume]);

  const isMutedRef = useRef(isMuted);
  useEffect(() => {
    isMutedRef.current = isMuted;
  }, [isMuted]);

  const timerOptionRef = useRef(timerOption);
  useEffect(() => {
    timerOptionRef.current = timerOption;
  }, [timerOption]);

  // Set timer option
  const startTimer = useCallback(
    (option: SleepTimerOption) => {
      setTimerOption(option);
      if (option > 0) {
        setRemainingSeconds(option * 60);
      } else {
        setRemainingSeconds(null);
        // Restore nominal volume immediately if cancelling active fadeout
        if (audioElement && !isMutedRef.current) {
          audioElement.volume = nominalVolumeRef.current;
        }
      }
    },
    [audioElement]
  );

  const hasActiveCountdown = timerOption > 0 && remainingSeconds !== null;

  // Countdown timer ticker
  useEffect(() => {
    if (!hasActiveCountdown) return;

    const interval = setInterval(() => {
      setRemainingSeconds((prev) => {
        if (prev === null || prev <= 1) {
          // Timer reached 0: pause playback and restore nominal volume
          if (audioElement) {
            onPause();
            if (!isMutedRef.current) {
              audioElement.volume = nominalVolumeRef.current;
            }
          }
          setTimerOption(0);
          return null;
        }

        const next = prev - 1;

        // Smoothly fade out volume over the last 10 seconds
        if (next <= 10 && audioElement && !isMutedRef.current) {
          const fadeFactor = Math.max(0, next / 10);
          audioElement.volume = nominalVolumeRef.current * fadeFactor;
        }

        return next;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [hasActiveCountdown, audioElement, onPause]);

  // Handle 'End of Track' option
  useEffect(() => {
    if (!audioElement) return;

    const onEnded = () => {
      if (timerOptionRef.current === -1) {
        onPause();
        setTimerOption(0);
        setRemainingSeconds(null);
      }
    };

    audioElement.addEventListener('ended', onEnded);
    return () => {
      audioElement.removeEventListener('ended', onEnded);
    };
  }, [audioElement, onPause]);

  // Format remaining time as mm:ss
  const formattedRemaining =
    remainingSeconds !== null
      ? `${Math.floor(remainingSeconds / 60)}:${String(remainingSeconds % 60).padStart(2, '0')}`
      : null;

  return {
    timerOption,
    remainingSeconds,
    formattedRemaining,
    startTimer,
  };
}
