// Regenerate the self-hosted HTTP/mobile background from our actual 3D scene.
// Start Vite and a separate Chrome with --remote-debugging-port=9223 first.
// Usage: node scripts/capture-ocean.mjs [http://127.0.0.1:5185] [9223]
import { mkdirSync, writeFileSync } from 'node:fs'

const origin = process.argv[2] || 'http://127.0.0.1:5185'
const cdp = `http://127.0.0.1:${process.argv[3] || '9223'}`
const target = await (await fetch(`${cdp}/json/new?${encodeURIComponent(`${origin}/backgrounds/abyssal.html`)}`, { method: 'PUT' })).json()
const socket = new WebSocket(target.webSocketDebuggerUrl)
await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject })
let sequence = 0
const pending = new Map()
socket.onmessage = event => {
  const result = JSON.parse(event.data)
  if (result.id) { pending.get(result.id)?.(result); pending.delete(result.id) }
}
const call = (method, params = {}) => new Promise((resolve, reject) => {
  const id = ++sequence
  pending.set(id, result => result.error ? reject(new Error(JSON.stringify(result.error))) : resolve(result.result))
  socket.send(JSON.stringify({ id, method, params }))
})
const evaluate = async expression => {
  const result = await call('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails))
  return result.result.value
}
try {
  await call('Emulation.setDeviceMetricsOverride', { width: 1600, height: 900, deviceScaleFactor: 1, mobile: false })
  const deadline = Date.now() + 120000
  while (true) {
    const status = await evaluate('document.documentElement.dataset.scene')
    if (status === 'ready') break
    if (status === 'error' || Date.now() > deadline) throw new Error(`Ocean renderer not ready: ${status}`)
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  const output = new URL('../public/backgrounds/', import.meta.url)
  mkdirSync(output, { recursive: true })
  const shot = await call('Page.captureScreenshot', { format: 'webp', quality: 88 })
  writeFileSync(new URL('abyssal-poster.webp', output), Buffer.from(shot.data, 'base64'))
  console.log('Scene ready; recording 22 seconds of fixed-camera animation.')
  const recording = await evaluate(`new Promise((resolve, reject) => {
    const stream = document.getElementById('c').captureStream(24);
    const recorder = new MediaRecorder(stream, { mimeType: 'video/webm;codecs=vp9', videoBitsPerSecond: 2200000 });
    const chunks = [];
    recorder.ondataavailable = event => chunks.push(event.data);
    recorder.onerror = reject;
    recorder.onstop = () => {
      stream.getTracks().forEach(track => track.stop());
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.readAsDataURL(new Blob(chunks, { type: 'video/webm' }));
    };
    recorder.start();
    setTimeout(() => recorder.stop(), 22000);
  })`)
  const bytes = Buffer.from(recording.split(',')[1], 'base64')
  writeFileSync(new URL('abyssal-loop.webm', output), bytes)
  console.log(`Saved scene recording: ${(bytes.length / 1024 / 1024).toFixed(2)} MiB`)
} finally {
  socket.close()
  await fetch(`${cdp}/json/close/${target.id}`)
}
