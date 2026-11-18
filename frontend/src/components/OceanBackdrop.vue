<template>
  <div class="ocean-backdrop" aria-hidden="true">
    <img class="ocean-layer" :src="posterUrl" alt="" />
    <video
      v-if="!reducedMotion && !saveData"
      ref="video"
      class="ocean-layer"
      :class="{ 'ocean-layer--hidden': sceneReady }"
      :src="videoUrl"
      :poster="posterUrl"
      autoplay muted loop playsinline preload="metadata"
    />
    <iframe
      v-if="useWebGPU"
      ref="scene"
      class="ocean-layer ocean-scene"
      :class="{ 'ocean-scene--ready': sceneReady }"
      :src="sceneUrl"
      title="Underwater background"
      tabindex="-1"
    />
    <div class="ocean-shade" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const base = `${import.meta.env.BASE_URL}backgrounds/`
const posterUrl = `${base}abyssal-poster.webp`
const videoUrl = `${base}abyssal-loop.webm`
const sceneUrl = `${base}abyssal.html`
const scene = ref<HTMLIFrameElement>()
const video = ref<HTMLVideoElement>()
const useWebGPU = ref(false)
const sceneReady = ref(false)
const reducedMotion = ref(true)
const saveData = ref(false)
let motion: MediaQueryList | undefined
let startup: ReturnType<typeof setTimeout> | undefined
let deadline: ReturnType<typeof setTimeout> | undefined

function syncPlayback() {
  if (document.hidden || reducedMotion.value || sceneReady.value) video.value?.pause()
  else video.value?.play().catch(() => { /* Poster remains when autoplay is unavailable. */ })
}
function fallback() {
  clearTimeout(deadline)
  scene.value?.contentWindow?.postMessage({ type: 'sensedeep-ocean-stop' }, window.location.origin)
  sceneReady.value = false
  useWebGPU.value = false
  syncPlayback()
}
function receive(event: MessageEvent) {
  if (event.origin !== window.location.origin || event.source !== scene.value?.contentWindow) return
  if (event.data?.type !== 'sensedeep-ocean') return
  if (event.data.status === 'ready') {
    clearTimeout(deadline)
    sceneReady.value = true
    syncPlayback()
  } else if (event.data.status === 'error') fallback()
}
function motionChanged() {
  reducedMotion.value = motion?.matches ?? true
  if (reducedMotion.value) fallback()
  syncPlayback()
}
onMounted(() => {
  motion = matchMedia('(prefers-reduced-motion: reduce)')
  reducedMotion.value = motion.matches
  const capabilities = navigator as Navigator & { deviceMemory?: number; connection?: { saveData?: boolean } }
  saveData.value = capabilities.connection?.saveData ?? false
  motion.addEventListener('change', motionChanged)
  document.addEventListener('visibilitychange', syncPlayback)
  window.addEventListener('message', receive)
  // HTTP LAN, mobile and smaller devices play a locally rendered recording of
  // this exact 3D scene. WebGPU needs HTTPS (localhost is a browser exception).
  if (window.isSecureContext && 'gpu' in navigator && !matchMedia('(pointer:coarse)').matches
    && (capabilities.deviceMemory ?? 8) >= 8 && !saveData.value && !reducedMotion.value) {
    startup = setTimeout(() => {
      if (reducedMotion.value) return
      useWebGPU.value = true
      deadline = setTimeout(fallback, 45000)
    }, 700)
  }
})
onBeforeUnmount(() => {
  clearTimeout(startup)
  clearTimeout(deadline)
  motion?.removeEventListener('change', motionChanged)
  document.removeEventListener('visibilitychange', syncPlayback)
  window.removeEventListener('message', receive)
  video.value?.pause()
  scene.value?.contentWindow?.postMessage({ type: 'sensedeep-ocean-stop' }, window.location.origin)
  // Removing the iframe tears down its isolated animation and GPU context.
})
</script>

<style scoped>
.ocean-backdrop { position: fixed; inset: 0; overflow: hidden; pointer-events: none; background: #062c35; }
.ocean-layer { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: 38% center; border: 0; }
.ocean-layer--hidden { opacity: 0; }
.ocean-scene { opacity: 0; transition: opacity 1.2s ease; }
.ocean-scene--ready { opacity: 1; }
.ocean-shade { position: absolute; inset: 0; background: linear-gradient(90deg, rgba(1, 19, 26, .08) 30%, rgba(1, 19, 26, .36)), linear-gradient(0deg, rgba(1, 18, 24, .68), transparent 55%, rgba(1, 18, 24, .24)); }
@media (prefers-reduced-motion: reduce) { .ocean-scene { transition: none; } }
</style>
