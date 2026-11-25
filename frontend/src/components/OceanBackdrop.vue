<template>
  <div class="ocean-backdrop" aria-hidden="true">
    <img class="ocean-layer" :src="posterUrl" alt="" />
    <video
      v-if="!reducedMotion && !saveData"
      ref="video"
      class="ocean-layer"
      :src="videoUrl"
      :poster="posterUrl"
      autoplay muted loop playsinline preload="metadata"
    />
    <div class="ocean-shade" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const base = `${import.meta.env.BASE_URL}backgrounds/`
const posterUrl = `${base}abyssal-poster.webp`
const videoUrl = `${base}abyssal-loop.webm`
const video = ref<HTMLVideoElement>()
const reducedMotion = ref(true)
const saveData = ref(false)
let motion: MediaQueryList | undefined

function syncPlayback() {
  if (document.hidden || reducedMotion.value) video.value?.pause()
  else video.value?.play().catch(() => { /* Poster remains when autoplay is unavailable. */ })
}
function motionChanged() {
  reducedMotion.value = motion?.matches ?? true
  syncPlayback()
}
onMounted(() => {
  motion = matchMedia('(prefers-reduced-motion: reduce)')
  reducedMotion.value = motion.matches
  const capabilities = navigator as Navigator & { connection?: { saveData?: boolean } }
  saveData.value = capabilities.connection?.saveData ?? false
  motion.addEventListener('change', motionChanged)
  document.addEventListener('visibilitychange', syncPlayback)
})
onBeforeUnmount(() => {
  motion?.removeEventListener('change', motionChanged)
  document.removeEventListener('visibilitychange', syncPlayback)
  video.value?.pause()
})
</script>

<style scoped>
.ocean-backdrop { position: fixed; inset: 0; overflow: hidden; pointer-events: none; background: #062c35; }
.ocean-layer { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: 38% center; border: 0; }
.ocean-shade { position: absolute; inset: 0; background: linear-gradient(90deg, rgba(1, 19, 26, .08) 30%, rgba(1, 19, 26, .36)), linear-gradient(0deg, rgba(1, 18, 24, .68), transparent 55%, rgba(1, 18, 24, .24)); }
</style>
