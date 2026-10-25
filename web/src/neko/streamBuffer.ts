export function supportsStreamBuffer() {
  return typeof RTCRtpReceiver !== 'undefined' && 'jitterBufferTarget' in RTCRtpReceiver.prototype
}

export function normalizeStreamBuffer(value: number | null): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(4000, value)) : null
}

export function applyStreamBuffer(receiver: RTCRtpReceiver, milliseconds: number | null) {
  if (!('jitterBufferTarget' in receiver)) return
  // Buffering is optional: a browser rejecting the preference must not
  // prevent the stream from playing.
  try { receiver.jitterBufferTarget = milliseconds } catch { /* Keep browser defaults. */ }
}
