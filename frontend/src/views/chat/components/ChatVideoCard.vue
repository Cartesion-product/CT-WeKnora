<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatVideoDuration, getVideoDetail, getVideoKeyframeUrl, getVideoPlaybackUrl } from '@/api/knowledge-base/video'
import type { ChatVideoReference } from '@/utils/chatVideoReferences'

const props = defineProps<{ video: ChatVideoReference }>()
const { t } = useI18n()
const card = ref<HTMLDivElement | null>(null)
const player = ref<HTMLVideoElement | null>(null)
const playbackUrl = ref('')
const posterUrl = ref('')
const posterFrameId = ref('')
const loading = ref(false)
const playing = ref(false)
const error = ref(false)
const staleSource = ref(false)
const durationMs = ref<number | undefined>()
let generation = 0
let expiresAt = 0
let renewTimer: ReturnType<typeof setTimeout> | undefined
let restore: { time: number; playing: boolean } | null = null
let visibilityObserver: IntersectionObserver | undefined
let visible = false
let pendingPlay = false

function scheduleRenew(current: number) {
  if (renewTimer) clearTimeout(renewTimer)
  if (playing.value && Number.isFinite(expiresAt)) {
    renewTimer = setTimeout(() => void refreshGrant(current), Math.max(10_000, expiresAt - Date.now() - 60_000))
  }
}

async function refreshGrant(current: number): Promise<boolean> {
  const previous = player.value
  const resume = previous && playing.value
    ? { time: previous.currentTime, playing: !previous.paused }
    : null
  try {
    const grant = await getVideoPlaybackUrl(props.video.knowledgeId)
    if (current !== generation) return false
    expiresAt = grant.expiresAt
    restore = resume
    playbackUrl.value = grant.url
    if (posterFrameId.value) posterUrl.value = getVideoKeyframeUrl(props.video.knowledgeId, posterFrameId.value, grant.token)
    error.value = false
    scheduleRenew(current)
    return true
  } catch {
    if (current === generation) {
      error.value = true
      if (expiresAt <= Date.now()) playbackUrl.value = ''
    }
    return false
  }
}

async function load() {
  const current = ++generation
  if (renewTimer) clearTimeout(renewTimer)
  playbackUrl.value = ''
  posterUrl.value = ''
  posterFrameId.value = ''
  loading.value = true
  playing.value = false
  error.value = false
  staleSource.value = false
  durationMs.value = undefined
  expiresAt = 0
  restore = null
  pendingPlay = false
  const id = props.video.knowledgeId
  if (!id) { loading.value = false; return }
  try {
    const [detail, grant] = await Promise.all([
      getVideoDetail(id, props.video.segmentId ? { segmentId: props.video.segmentId } : {})
        .catch(() => getVideoDetail(id).catch(() => null)),
      getVideoPlaybackUrl(id),
    ])
    if (current !== generation) return
    staleSource.value = !!props.video.processingRunId && !!detail?.asset?.active_run_id
      && props.video.processingRunId !== detail.asset.active_run_id
    durationMs.value = detail?.asset?.duration_ms
    const frames = detail?.keyframes || []
    const frame = frames.find(item => item.segment_id === props.video.segmentId)
      || frames.reduce<(typeof frames)[number] | undefined>((closest, item) =>
        !closest || Math.abs(item.frame_ms - (props.video.startMs ?? 0)) < Math.abs(closest.frame_ms - (props.video.startMs ?? 0))
          ? item : closest, undefined)
    if (frame) {
      posterFrameId.value = frame.id
      posterUrl.value = getVideoKeyframeUrl(id, frame.id, grant.token)
    }
    playbackUrl.value = grant.url
    expiresAt = grant.expiresAt
  } catch {
    if (current === generation) error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

function onLoadedMetadata() {
  if (!player.value) return
  const duration = player.value.duration
  if (Number.isFinite(duration)) durationMs.value = Math.round(duration * 1000)
  const requestedTime = restore?.time ?? (staleSource.value ? 0 : Math.max(0, props.video.startMs ?? 0) / 1000)
  player.value.currentTime = Number.isFinite(duration) && requestedTime >= duration ? 0 : requestedTime
  if (restore?.playing || pendingPlay) void player.value.play().catch(() => { error.value = true })
  restore = null
  pendingPlay = false
}

async function play() {
  if (!playbackUrl.value || (Number.isFinite(expiresAt) && expiresAt <= Date.now() + 10_000)) {
    if (!await refreshGrant(generation)) return
  }
  if (!playbackUrl.value) return
  pendingPlay = true
  playing.value = true
  scheduleRenew(generation)
  await nextTick()
  if (player.value && player.value.readyState >= HTMLMediaElement.HAVE_METADATA) onLoadedMetadata()
}

function pauseOtherVideos() {
  document.querySelectorAll<HTMLVideoElement>('.answer-video-references video').forEach(video => {
    if (video !== player.value) video.pause()
  })
}

watch(() => [props.video.knowledgeId, props.video.segmentId, props.video.startMs], () => {
  if (visible) void load()
})
onMounted(() => {
  if (!card.value || typeof IntersectionObserver === 'undefined') {
    visible = true
    void load()
    return
  }
  visibilityObserver = new IntersectionObserver(entries => {
    if (!entries[0]?.isIntersecting) return
    visible = true
    visibilityObserver?.disconnect()
    void load()
  }, { rootMargin: '300px' })
  visibilityObserver.observe(card.value)
})
onBeforeUnmount(() => {
  generation++
  visibilityObserver?.disconnect()
  if (renewTimer) clearTimeout(renewTimer)
})
</script>

<template>
  <div ref="card" class="chat-video-card">
    <div class="chat-video-card__screen">
      <video v-if="playing && playbackUrl" ref="player" :src="playbackUrl" :poster="posterUrl || undefined"
        controls playsinline preload="metadata" @loadedmetadata="onLoadedMetadata"
        @play="pauseOtherVideos" @error="error = true" />
      <img v-else-if="posterUrl" class="chat-video-card__poster" :src="posterUrl" alt="" loading="lazy" />
      <div v-else class="chat-video-card__placeholder"><t-icon name="video" size="28px" /></div>
      <button v-if="!playing" type="button" class="chat-video-card__play" :disabled="loading"
        :aria-label="`${t('videoKnowledge.play')} ${video.name}`" @click="play">
        <t-icon :name="loading ? 'loading' : 'play-circle'" size="32px" />
      </button>
      <span v-if="!playing && durationMs" class="chat-video-card__time" :title="t('videoKnowledge.duration')">{{ formatVideoDuration(durationMs) }}</span>
    </div>
    <div class="chat-video-card__info">
      <span class="chat-video-card__name" :title="video.name">{{ video.name }}</span>
      <span v-if="video.startMs && !staleSource && (!durationMs || video.startMs < durationMs)" class="chat-video-card__position">{{ t('videoKnowledge.playFrom') }} {{ formatVideoDuration(video.startMs) }}</span>
      <span v-if="video.snippet" class="chat-video-card__snippet" :title="video.snippet">{{ video.snippet }}</span>
      <span v-if="staleSource" class="chat-video-card__warning">{{ t('videoKnowledge.sourceUpdated') }}</span>
      <button v-if="error" type="button" class="chat-video-card__retry" @click="load">{{ t('videoKnowledge.retry') }}</button>
    </div>
  </div>
</template>

<style scoped>
.chat-video-card { min-width: 0; overflow: hidden; border: 1px solid var(--td-component-stroke); border-radius: 6px; background: var(--td-bg-color-container); }
.chat-video-card__screen { position: relative; aspect-ratio: 16 / 9; background: #11262b; overflow: hidden; }
.chat-video-card__screen video { display: block; width: 100%; height: 100%; object-fit: contain; }
.chat-video-card__poster { display: block; width: 100%; height: 100%; object-fit: contain; }
.chat-video-card__placeholder { display: grid; place-items: center; width: 100%; height: 100%; color: rgba(255,255,255,.6); }
.chat-video-card__play { position: absolute; inset: 0; display: grid; place-items: center; width: 100%; border: 0; color: #fff; background: rgba(4,29,33,.18); cursor: pointer; }
.chat-video-card__play :deep(.t-icon) { filter: drop-shadow(0 1px 6px rgba(0,0,0,.6)); }
.chat-video-card__play:hover { background: rgba(4,29,33,.32); }
.chat-video-card__play:disabled { cursor: wait; }
.chat-video-card__time { position: absolute; right: 8px; bottom: 8px; padding: 2px 5px; border-radius: 3px; color: #fff; background: rgba(0,0,0,.7); font-size: 12px; pointer-events: none; }
.chat-video-card__info { display: flex; flex-direction: column; gap: 3px; padding: 8px 10px 10px; }
.chat-video-card__name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; font-weight: 600; }
.chat-video-card__snippet { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--td-text-color-secondary); font-size: 12px; }
.chat-video-card__position { color: var(--td-text-color-secondary); font-size: 12px; }
.chat-video-card__warning { color: var(--td-warning-color); font-size: 12px; }
.chat-video-card__retry { align-self: flex-start; padding: 0; border: 0; background: transparent; color: var(--td-brand-color); font-size: 12px; cursor: pointer; }
.chat-video-card__play:focus-visible, .chat-video-card__retry:focus-visible { outline: 2px solid var(--td-brand-color); outline-offset: -3px; }
</style>
