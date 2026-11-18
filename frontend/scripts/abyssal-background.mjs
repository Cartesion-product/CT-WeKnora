// Adaptation of Noniv/abyssal-garden (MIT); see vendor/abyssal-garden/README.md.
import { readFileSync } from 'node:fs'

const root = new URL('../vendor/abyssal-garden/', import.meta.url)
const read = (file) => readFileSync(new URL(file, root), 'utf8')

function replaceOnce(source, from, to) {
  if (!source.includes(from)) throw new Error(`Abyssal source changed: ${from.slice(0, 80)}`)
  return source.replace(from, to)
}

export function backgroundHtml() {
  // Leave the upstream geometry and WGSL untouched. Replace only the interactive shell.
  let main = read('source/src/main.js')
  main = main.slice(main.indexOf('const ctx ='), main.indexOf('// CS[j]'))
  // A stationary camera never needs the upstream collision brick field.
  const collisionStart = main.indexOf('// Distance to the rock for collide()')
  const collisionEnd = main.indexOf("await say('Carving canyons")
  if (collisionStart < 0 || collisionEnd < collisionStart) throw new Error('Abyssal collision markers changed')
  main = main.slice(0, collisionStart) + main.slice(collisionEnd)
  main = replaceOnce(main, "keep('abyssalLoad', 1);", '')
  main = replaceOnce(main, "addEventListener('pagehide', () => keep('abyssalLoad', ''));", '')
  main = replaceOnce(main, 'const PIXMAX = 2560 * 1440', 'const PIXMAX = 1280 * 720')
  main = replaceOnce(main, "addEventListener('resize', resize);", '')
  main = replaceOnce(main, 'const Wd = makeWorld(D, T, flyPts, POSES);', `
const Wd = makeWorld(D, T, flyPts, POSES);
// Keep the same deterministic environment, with fewer animated instances.
const thin = (data, stride, step) => {
  const result = new Float32Array(Math.ceil(data.length / stride / step) * stride);
  for (let i = 0, j = 0; i < data.length; i += stride * step, j += stride) result.set(data.subarray(i, i + stride), j);
  return result;
};
Wd.fish = thin(Wd.fish, 8, 4);
Wd.mantas = thin(Wd.mantas, 8, 3);
Wd.bubbles = thin(Wd.bubbles, 8, 3);
Wd.snow = thin(Wd.snow, 4, 4);
Wd.glints = thin(Wd.glints, 8, 2);
`)
  const quality = `
const mob = matchMedia('(pointer:coarse)').matches;
const SV = [], LV = mob ? 2 : 1;
const THIN = 1 << LV, SHADOW = 2048, AA = LV > 1 ? 1 : 4;
const WATER = LV > 1 ? 256 : 512, RAYN = LV > 1 ? 12 : 34;
`
  const setup = `
const canvas = document.getElementById('c');
const say = () => new Promise(resolve => setTimeout(resolve, 0));
let stopped = false, raf = 0;
if (!navigator.gpu) throw new Error('WebGPU unavailable');
const adapter = await navigator.gpu.requestAdapter({powerPreference: 'low-power'});
if (!adapter) throw new Error('WebGPU adapter unavailable');
const hasTS = adapter.features.has('timestamp-query');
const device = await adapter.requestDevice({requiredFeatures: hasTS ? ['timestamp-query'] : []});
const stop = () => { stopped = true; cancelAnimationFrame(raf); device.destroy(); };
addEventListener('pagehide', stop, {once: true});
addEventListener('message', event => {
  if (event.source === parent && event.origin === location.origin && event.data?.type === 'sensedeep-ocean-stop') stop();
});
device.addEventListener('uncapturederror', e => { report('error'); console.warn('Ocean GPU:', e.error.message); stop(); });
device.lost.then(info => { if (info.reason !== 'destroyed') { report('error'); stop(); } });
`
  const loop = `
// A fixed viewpoint: only water, plants, wildlife and light animate.
lookAt(...POSES[0]);
F[TIME] = 12;
render(); await Q.onSubmittedWorkDone();
if (stopped) return;
report('ready');
const motion = matchMedia('(prefers-reduced-motion: reduce)');
let last = 0, samples = 0, elapsed = 0;
function frame(now) {
  if (stopped || document.hidden || motion.matches) return;
  raf = requestAnimationFrame(frame);
  if (now - last < 1000 / 24) return;
  const dt = last ? Math.min((now - last) / 1000, .15) : 0;
  last = now;
  F[TIME] += dt;
  render();
  // Lower resolution on sustained slow GPUs, without changing the fixed framing.
  if (++samples >= 90) {
    if (elapsed > 5.5 && pix > PIXMIN) { pix = Math.max(PIXMIN, pix * .75); resize(); }
    samples = elapsed = 0;
  }
  elapsed += dt;
}
function resume() {
  cancelAnimationFrame(raf); last = 0;
  if (!stopped && !document.hidden && !motion.matches) raf = requestAnimationFrame(frame);
}
document.addEventListener('visibilitychange', resume);
motion.addEventListener('change', resume);
addEventListener('resize', () => { if (!stopped) { resize(); render(); } });
resume();
`
  const code = [quality, read('source/src/math.js'), read('source/src/shaders.js'), read('source/src/world.js'), setup, main, loop].join('\n')
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>SenseDeep ocean</title><style>html,body{margin:0;width:100%;height:100%;overflow:hidden;background:#062c35}canvas{display:block;width:100%;height:100%;pointer-events:none}</style></head><body><canvas id="c" aria-hidden="true"></canvas><script>
/* ${read('LICENSE').replaceAll('*/', '* /')} */
const report = status => { document.documentElement.dataset.scene = status; parent.postMessage({type:'sensedeep-ocean',status}, location.origin); };
(async () => {${code}\n})().catch(error => { console.warn('Ocean background:', error); report('error'); });
</script></body></html>`
}

/** Same standalone, self-hosted asset in Vite dev and the production build. */
export function abyssalBackground() {
  return {
    name: 'sensedeep-abyssal-background',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (req.url?.split('?')[0]?.endsWith('/backgrounds/abyssal.html')) {
          res.setHeader('Content-Type', 'text/html; charset=utf-8')
          res.end(backgroundHtml())
        } else next()
      })
    },
    generateBundle() {
      this.emitFile({type: 'asset', fileName: 'backgrounds/abyssal.html', source: backgroundHtml()})
      this.emitFile({type: 'asset', fileName: 'backgrounds/abyssal-LICENSE.txt', source: read('LICENSE')})
    },
  }
}
