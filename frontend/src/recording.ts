/**
 * Browser-side session recording.
 *
 * The workspace already pulls screenshots from the session host at the
 * user-selected FPS. Recording reuses those exact frames instead of capturing
 * or fetching anything extra: every received frame is drawn to an offscreen
 * canvas and handed to a MediaRecorder. A 1 FPS session therefore produces a
 * 1 FPS recording, with no extra capture, network or encoding workload.
 */

const RECORDING_MIME_TYPES = [
  'video/webm;codecs=vp9',
  'video/webm;codecs=vp8',
  'video/webm',
  'video/mp4',
];

export interface SessionRecorder {
  /** Epoch milliseconds when the recording started. */
  readonly startedAt: number;
  /** Mime type of the produced blob. */
  readonly mimeType: string;
  /** Record the frame currently shown by the session image. */
  drawFrame(source: HTMLImageElement): void;
  /** Flush the encoder and resolve the finished recording. */
  stop(): Promise<Blob>;
}

type FrameTrack = MediaStreamTrack & { requestFrame?: () => void };

export function pickRecordingMimeType(): string {
  if (typeof MediaRecorder === 'undefined' || typeof MediaRecorder.isTypeSupported !== 'function') {
    return '';
  }
  return RECORDING_MIME_TYPES.find((type) => MediaRecorder.isTypeSupported(type)) || '';
}

/** Formats elapsed milliseconds as mm:ss (or h:mm:ss past an hour). */
export function formatRecordingTime(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const mm = String(minutes).padStart(2, '0');
  const ss = String(seconds).padStart(2, '0');
  return hours > 0 ? `${hours}:${mm}:${ss}` : `${mm}:${ss}`;
}

/**
 * Creates a recorder for the given image element. Returns null when the
 * browser cannot record (no MediaRecorder / canvas capture support).
 */
export function createSessionRecorder(source: HTMLImageElement, fps: number): SessionRecorder | null {
  if (typeof MediaRecorder === 'undefined' || typeof document === 'undefined') {
    return null;
  }

  const width = source.naturalWidth || source.width;
  const height = source.naturalHeight || source.height;
  if (!width || !height) {
    return null;
  }

  const canvas = document.createElement('canvas');
  if (typeof canvas.captureStream !== 'function') {
    return null;
  }
  canvas.width = width;
  canvas.height = height;

  const ctx = canvas.getContext('2d');
  if (!ctx) {
    return null;
  }
  ctx.drawImage(source, 0, 0);

  // Prefer an on-demand capture stream: exactly one encoded frame per frame
  // received from the session host (true 1 FPS when the stream is 1 FPS).
  // Browsers without requestFrame fall back to a fixed-FPS capture stream.
  let stream: MediaStream;
  let frameTrack: FrameTrack | null = null;
  const fallbackFps = Math.max(1, Math.round(fps) || 1);
  try {
    stream = canvas.captureStream(0);
    const track = stream.getVideoTracks()[0] as FrameTrack | undefined;
    if (track && typeof track.requestFrame === 'function') {
      frameTrack = track;
    } else {
      stream.getTracks().forEach((t) => t.stop());
      stream = canvas.captureStream(fallbackFps);
    }
  } catch {
    stream = canvas.captureStream(fallbackFps);
  }

  const mimeType = pickRecordingMimeType();
  let recorder: MediaRecorder;
  try {
    recorder = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream);
  } catch {
    try {
      recorder = new MediaRecorder(stream);
    } catch {
      stream.getTracks().forEach((t) => t.stop());
      return null;
    }
  }

  const chunks: Blob[] = [];
  recorder.ondataavailable = (event: BlobEvent) => {
    if (event.data && event.data.size > 0) {
      chunks.push(event.data);
    }
  };
  try {
    recorder.start(1000);
  } catch {
    recorder.start();
  }

  if (frameTrack) {
    frameTrack.requestFrame?.();
  }

  const startedAt = Date.now();
  let stopped: Promise<Blob> | null = null;

  return {
    startedAt,
    mimeType: recorder.mimeType || mimeType || 'video/webm',
    drawFrame(image: HTMLImageElement) {
      ctx.drawImage(image, 0, 0);
      frameTrack?.requestFrame?.();
    },
    stop() {
      if (stopped) {
        return stopped;
      }
      stopped = new Promise<Blob>((resolve) => {
        recorder.onstop = () => {
          stream.getTracks().forEach((t) => t.stop());
          const type = recorder.mimeType || mimeType || 'video/webm';
          resolve(new Blob(chunks, { type }));
        };
        try {
          recorder.stop();
        } catch {
          stream.getTracks().forEach((t) => t.stop());
          resolve(new Blob(chunks, { type: recorder.mimeType || mimeType || 'video/webm' }));
        }
      });
      return stopped;
    },
  };
}
