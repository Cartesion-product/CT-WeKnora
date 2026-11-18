<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getVideoDetail, getVideoStatus, getVideoPlaybackUrl, getVideoKeyframeUrl, type VideoDetail, type VideoSegment } from '@/api/knowledge-base/video'

const props = defineProps<{ knowledgeId: string; active: boolean; startMs?: number; segmentId?: string; processingRunId?: string }>()
const route = useRoute()
const { t } = useI18n()
const player = ref<HTMLVideoElement | null>(null)
const detail = ref<VideoDetail | null>(null)
const loading = ref(false)
const error = ref('')
const playbackUrl = ref('')
const playbackToken = ref('')
const playbackError = ref('')
const currentMs = ref(0)
const nextOffset = ref<number | null>(null)
const moreLoading = ref(false)
const moreError = ref('')
const processRun = ref<VideoDetail['run']>(null)
let generation = 0
let tokenTimer: ReturnType<typeof setTimeout> | undefined
let statusTimer: ReturnType<typeof setTimeout> | undefined
let restorePlayback: { time: number; playing: boolean } | null = null

const segments = computed(() => [...(detail.value?.segments || [])].sort((a, b) => a.start_ms - b.start_ms))
const activeSegmentId = computed(() => segments.value.find(segment => currentMs.value >= segment.start_ms && currentMs.value < segment.end_ms)?.segment_id)
const sourceRunId = computed(() => props.processingRunId || (route.query.knowledge_id === props.knowledgeId && typeof route.query.processing_run_id === 'string' ? route.query.processing_run_id : ''))
const staleSource = computed(() => !!sourceRunId.value && !!detail.value?.asset?.active_run_id && sourceRunId.value !== detail.value.asset.active_run_id)

function formatTime(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '00:00'
  const seconds = Math.floor(ms / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  return hours ? `${hours}:${String(minutes % 60).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
    : `${minutes}:${String(seconds % 60).padStart(2, '0')}`
}

function seek(ms: number) {
  if (!player.value || !Number.isFinite(ms)) return
  player.value.currentTime = Math.max(0, ms) / 1000
  currentMs.value = ms
}

function seekSegment(segment: VideoSegment) {
  seek(segment.start_ms)
}

function evidenceFor(segment: VideoSegment) {
  const id = segment.segment_id
  return (detail.value?.evidences || []).filter(evidence => evidence.segment_id === id)
}

function framesFor(segment: VideoSegment) {
  const id = segment.segment_id
  return (detail.value?.keyframes || []).filter(frame => frame.segment_id === id)
}

function thumbnailUrl(frameId: string): string {
  return playbackToken.value ? getVideoKeyframeUrl(props.knowledgeId, frameId, playbackToken.value) : ''
}

function targetFromRoute() {
  if (staleSource.value) return
  if (props.segmentId || Number.isFinite(props.startMs)) {
    const segment = segments.value.find(item => item.segment_id === props.segmentId)
    if (props.segmentId && !segment) return
    seek(props.startMs ?? segment?.start_ms ?? NaN)
    return
  }
  if (route.query.knowledge_id !== props.knowledgeId) return
  const segmentId = typeof route.query.segment_id === 'string' ? route.query.segment_id : ''
  const segment = segments.value.find(item => item.segment_id === segmentId)
  if (segmentId && !segment) return
  const rawMs = typeof route.query.start_ms === 'string' ? Number(route.query.start_ms) : NaN
  const target = Number.isFinite(rawMs) ? rawMs : segment?.start_ms
  if (typeof target === 'number' && Number.isFinite(target)) seek(target)
}

function referencedSegmentId(): string {
  if (props.segmentId) return props.segmentId
  return route.query.knowledge_id === props.knowledgeId && typeof route.query.segment_id === 'string'
    ? route.query.segment_id : ''
}

function appendPage(page: VideoDetail) {
  if (!detail.value) return
  const known = new Set(detail.value.segments.map(segment => segment.segment_id))
  detail.value.segments.push(...page.segments.filter(segment => !known.has(segment.segment_id)))
  const evidenceIds = new Set(detail.value.evidences.map(evidence => evidence.evidence_id))
  detail.value.evidences.push(...page.evidences.filter(evidence => !evidenceIds.has(evidence.evidence_id)))
  const frameIds = new Set(detail.value.keyframes.map(frame => frame.id))
  detail.value.keyframes.push(...page.keyframes.filter(frame => !frameIds.has(frame.id)))
}

async function loadReferencedSegment(currentGeneration: number) {
  const segmentId = referencedSegmentId()
  if (!segmentId || staleSource.value || segments.value.some(segment => segment.segment_id === segmentId)) return
  try {
    const page = await getVideoDetail(props.knowledgeId, { segmentId })
    if (currentGeneration !== generation || !detail.value || page.asset?.active_run_id !== detail.value.asset?.active_run_id) return
    appendPage(page)
    await nextTick()
    targetFromRoute()
  } catch {
    // The source may have been removed or rebuilt; keep the current page visible.
  }
}

async function loadMore() {
  const offset = nextOffset.value
  if (offset === null || moreLoading.value) return
  const currentGeneration = generation
  moreLoading.value = true
  moreError.value = ''
  try {
    const page = await getVideoDetail(props.knowledgeId, { offset })
    if (currentGeneration !== generation || !detail.value) return
    if (page.asset?.active_run_id !== detail.value.asset?.active_run_id) {
      await load()
      return
    }
    appendPage(page)
    nextOffset.value = page.next_offset ?? null
  } catch (cause: any) {
    if (currentGeneration === generation) moreError.value = cause?.message || t('videoKnowledge.loadFailed')
  } finally {
    if (currentGeneration === generation) moreLoading.value = false
  }
}

async function refreshPlayback(id: string, currentGeneration: number) {
  const element = player.value
  const time = element?.currentTime || 0
  const wasPlaying = element ? !element.paused : false
  try {
    const token = await getVideoPlaybackUrl(id)
    if (currentGeneration !== generation || !props.active || props.knowledgeId !== id) return
    playbackUrl.value = token.url
    playbackToken.value = token.token
    playbackError.value = ''
    restorePlayback = time > 0 ? { time, playing: wasPlaying } : null
    if (tokenTimer) clearTimeout(tokenTimer)
    if (Number.isFinite(token.expiresAt)) {
      tokenTimer = setTimeout(() => void refreshPlayback(id, currentGeneration), Math.max(10_000, token.expiresAt - Date.now() - 60_000))
    }
  } catch (cause: any) {
    if (currentGeneration === generation) playbackError.value = cause?.message || t('videoKnowledge.playbackFailed')
  }
}

function scheduleStatusPoll(id: string, currentGeneration: number) {
  if (statusTimer) clearTimeout(statusTimer)
  if (!props.active) return
  statusTimer = setTimeout(async () => {
    try {
      const status = await getVideoStatus(id)
      if (currentGeneration !== generation || !props.active || props.knowledgeId !== id || !detail.value) return
      const publishedChanged = status.asset?.active_run_id !== detail.value.asset?.active_run_id
      detail.value.asset = status.asset
      processRun.value = status.run
      if (publishedChanged) {
        await load()
        return
      }
    } catch {
      // A temporary status failure should not interrupt playback.
    }
    if (currentGeneration === generation) scheduleStatusPoll(id, currentGeneration)
  }, 5000)
}

function onLoadedMetadata() {
  if (restorePlayback && player.value) {
    player.value.currentTime = restorePlayback.time
    if (restorePlayback.playing) void player.value.play().catch(() => {})
    restorePlayback = null
    return
  }
  targetFromRoute()
}

async function load() {
  const currentGeneration = ++generation
  if (tokenTimer) clearTimeout(tokenTimer)
  if (statusTimer) clearTimeout(statusTimer)
  tokenTimer = undefined
  statusTimer = undefined
  detail.value = null
  processRun.value = null
  nextOffset.value = null
  moreError.value = ''
  playbackUrl.value = ''
  playbackToken.value = ''
  error.value = ''
  playbackError.value = ''
  restorePlayback = null
  if (!props.active || !props.knowledgeId) return
  loading.value = true
  try {
    const data = await getVideoDetail(props.knowledgeId)
    if (currentGeneration !== generation) return
    detail.value = data
    processRun.value = data.run
    nextOffset.value = data.next_offset ?? null
    await nextTick()
    targetFromRoute()
    void loadReferencedSegment(currentGeneration)
    void refreshPlayback(props.knowledgeId, currentGeneration)
    scheduleStatusPoll(props.knowledgeId, currentGeneration)
  } catch (cause: any) {
    if (currentGeneration === generation) error.value = cause?.message || t('videoKnowledge.loadFailed')
  } finally {
    if (currentGeneration === generation) loading.value = false
  }
}

watch(() => [props.knowledgeId, props.active], load, { immediate: true })
watch(() => [route.query.segment_id, route.query.start_ms, route.query.processing_run_id, props.startMs, props.segmentId, props.processingRunId], () => {
  void nextTick(() => {
    targetFromRoute()
    void loadReferencedSegment(generation)
  })
})
onBeforeUnmount(() => {
  generation++
  if (tokenTimer) clearTimeout(tokenTimer)
  if (statusTimer) clearTimeout(statusTimer)
})
</script>

<template>
  <div class="video-detail">
    <div v-if="loading" class="video-state"><t-loading size="small" /> {{ t('common.loading') }}</div>
    <div v-else-if="error" class="video-state video-state--error">
      <span>{{ error }}</span>
      <t-button size="small" variant="text" @click="load">{{ t('videoKnowledge.retry') }}</t-button>
    </div>
    <template v-else-if="detail">
      <div v-if="staleSource" class="video-state video-state--error" role="status">{{ t('videoKnowledge.sourceUpdated') }}</div>
      <div class="video-player-wrap">
        <video v-if="playbackUrl" ref="player" class="video-player" :src="playbackUrl" controls preload="metadata"
          @timeupdate="currentMs = ($event.target as HTMLVideoElement).currentTime * 1000"
          @loadedmetadata="onLoadedMetadata" @error="playbackError = t('videoKnowledge.playbackFailed')" />
        <div v-else class="video-state">{{ playbackError || t('videoKnowledge.loadingPlayback') }}</div>
      </div>
      <div v-if="playbackError && playbackUrl" class="video-state video-state--error">
        {{ playbackError }}
        <t-button size="small" variant="text" @click="refreshPlayback(knowledgeId, generation)">{{ t('videoKnowledge.retry') }}</t-button>
      </div>
      <div class="video-meta">
        <span>{{ t('videoKnowledge.duration') }}: {{ formatTime(detail.asset?.duration_ms || 0) }}</span>
        <span v-if="processRun?.stage || detail.asset?.status">{{ processRun?.stage || detail.asset?.status }}</span>
        <span v-if="processRun?.total_windows">{{ processRun.completed_windows || 0 }}/{{ processRun.total_windows }}</span>
      </div>
      <div v-if="detail.asset?.failure_code || processRun?.failure_code" class="video-state video-state--error">
        {{ processRun?.failure_code || detail.asset?.failure_code }}
      </div>
      <div class="video-segments-heading">{{ t('videoKnowledge.segments') }} <span>{{ segments.length }}</span></div>
      <div v-if="!segments.length" class="video-state">{{ t('videoKnowledge.noSegments') }}</div>
      <ol v-else class="video-segments">
        <li v-for="segment in segments" :key="segment.segment_id"
          :class="{ 'video-segment--active': activeSegmentId === segment.segment_id }">
          <button type="button" class="video-segment-time" @click="seekSegment(segment)"
            :title="t('videoKnowledge.seek')">
            {{ formatTime(segment.start_ms) }} - {{ formatTime(segment.end_ms) }}
          </button>
          <p v-if="segment.content || segment.text" class="video-segment-text">{{ segment.content || segment.text }}</p>
          <div v-for="evidence in evidenceFor(segment)" :key="evidence.evidence_id" class="video-evidence">
            <span class="video-modality">{{ t(`videoKnowledge.modality.${evidence.modality}`) }}</span>
            <button v-if="evidence.frame_ms !== undefined" type="button" class="video-frame-time"
              @click="seek(evidence.frame_ms)">{{ formatTime(evidence.frame_ms) }}</button>
            <span>{{ evidence.text || evidence.content }}</span>
          </div>
          <div v-if="framesFor(segment).length" class="video-frames">
            <button v-for="frame in framesFor(segment)" :key="frame.id" type="button"
              @click="seek(frame.frame_ms)" :title="t('videoKnowledge.seek')">
              <img v-if="thumbnailUrl(frame.id)" :src="thumbnailUrl(frame.id)" alt="" loading="lazy" />
              {{ t('videoKnowledge.frame') }} {{ formatTime(frame.frame_ms) }}
            </button>
          </div>
        </li>
      </ol>
      <div v-if="moreError" class="video-state video-state--error">{{ moreError }}</div>
      <div v-if="nextOffset !== null" class="video-more">
        <t-button variant="outline" :loading="moreLoading" @click="loadMore">{{ t('videoKnowledge.loadMore') }}</t-button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.video-detail { min-width: 0; }
.video-player-wrap { width: 100%; background: #111; aspect-ratio: 16 / 9; display: grid; place-items: center; }
.video-player { width: 100%; height: 100%; max-height: 55vh; }
.video-state { display: flex; align-items: center; gap: 8px; padding: 16px; color: #667085; }
.video-state--error { color: #b42318; }
.video-meta { display: flex; flex-wrap: wrap; gap: 14px; padding: 12px 0; color: #667085; font-size: 12px; }
.video-segments-heading { display: flex; gap: 8px; font-weight: 600; margin: 8px 0; }
.video-segments-heading span { color: #667085; font-weight: 400; }
.video-segments { list-style: none; margin: 0; padding: 0; }
.video-segments li { border-top: 1px solid #e6e8eb; padding: 12px 0; }
.video-segments li.video-segment--active { border-left: 3px solid #1677ff; padding-left: 9px; }
.video-segment-time, .video-frame-time, .video-frames button { border: 0; background: none; color: #1765ae; cursor: pointer; padding: 2px 0; }
.video-segment-time { font-weight: 600; }
.video-segment-text { margin: 8px 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.video-evidence { display: flex; flex-wrap: wrap; gap: 6px; margin: 7px 0; font-size: 13px; overflow-wrap: anywhere; }
.video-modality { color: #475467; font-size: 11px; text-transform: uppercase; }
.video-frames { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 8px; font-size: 12px; }
.video-frames button { display: flex; flex-direction: column; align-items: flex-start; width: 128px; }
.video-frames img { display: block; width: 128px; height: 72px; object-fit: cover; background: #111; margin-bottom: 4px; }
.video-more { display: flex; justify-content: center; padding: 16px; }
button:focus-visible { outline: 2px solid #1677ff; outline-offset: 2px; }
</style>
