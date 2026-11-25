// Regenerate the self-hosted login background from the actual 3D scene.
// Start Vite and a separate Chrome with --remote-debugging-port=9223 first.
// Usage: node scripts/capture-ocean.mjs [origin] [CDP port] [ffmpeg path]
import { mkdirSync, renameSync, writeFileSync } from 'node:fs'
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const origin = process.argv[2] || 'http://127.0.0.1:5185'
const cdp = `http://127.0.0.1:${process.argv[3] || '9223'}`
const ffmpegPath = process.argv[4] || process.env.FFMPEG_PATH || 'ffmpeg'
const fps = 30
const duration = 40
const output = new URL('../public/backgrounds/', import.meta.url)
const tempVideo = new URL('abyssal-loop.webm.tmp', output)
const finalVideo = new URL('abyssal-loop.webm', output)
const target = await (await fetch(`${cdp}/json/new?${encodeURIComponent(`${origin}/backgrounds/abyssal.html`)}`, { method: 'PUT' })).json()
const socket = new WebSocket(target.webSocketDebuggerUrl)
await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject })
let sequence = 0
const pending = new Map()
socket.onmessage = event => {
  const result = JSON.parse(event.data)
  if (result.id) { pending.get(result.id)?.(result); pending.delete(result.id) }
}
socket.onclose = () => {
  for (const callback of pending.values()) callback({ error: { message: 'Chrome capture tab disconnected' } })
  pending.clear()
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
  await call('Emulation.setDeviceMetricsOverride', { width: 1280, height: 720, deviceScaleFactor: 1, mobile: false })
  const deadline = Date.now() + 120000
  while (true) {
    const status = await evaluate('document.documentElement.dataset.scene')
    if (status === 'ready') break
    if (status === 'error' || Date.now() > deadline) throw new Error(`Ocean renderer not ready: ${status}`)
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  mkdirSync(output, { recursive: true })
  const shot = await call('Page.captureScreenshot', { format: 'webp', quality: 88 })
  writeFileSync(new URL('abyssal-poster.webp', output), Buffer.from(shot.data, 'base64'))

  const encoder = spawn(ffmpegPath, [
    '-hide_banner', '-loglevel', 'error', '-y',
    '-f', 'image2pipe', '-framerate', String(fps), '-vcodec', 'png', '-i', 'pipe:0',
    '-an', '-c:v', 'libvpx-vp9', '-b:v', '0', '-crf', '35',
    '-deadline', 'good', '-cpu-used', '4', '-row-mt', '1', '-pix_fmt', 'yuv420p',
    '-f', 'webm', fileURLToPath(tempVideo),
  ], { stdio: ['pipe', 'ignore', 'pipe'] })
  let encoderError = ''
  encoder.stderr.on('data', chunk => { encoderError += chunk.toString() })
  const encoded = new Promise((resolve, reject) => {
    encoder.on('error', reject)
    encoder.on('close', code => code === 0 ? resolve() : reject(new Error(`ffmpeg exited ${code}: ${encoderError}`)))
  })

  console.log(`Rendering ${duration * fps} frames at ${fps} fps (${duration} seconds).`)
  for (let i = 0; i < duration * fps; i++) {
    await evaluate(`captureOceanFrame(${12 + i / fps})`)
    const frame = await call('Page.captureScreenshot', { format: 'png' })
    const png = Buffer.from(frame.data, 'base64')
    await new Promise((resolve, reject) => encoder.stdin.write(png, error => error ? reject(error) : resolve()))
    if ((i + 1) % fps === 0) console.log(`Rendered ${(i + 1) / fps}/${duration} seconds`)
  }
  encoder.stdin.end()
  await encoded
  renameSync(tempVideo, finalVideo)
  console.log('Saved fixed-frame-rate ocean recording and poster.')
} finally {
  socket.close()
  await fetch(`${cdp}/json/close/${target.id}`)
}
