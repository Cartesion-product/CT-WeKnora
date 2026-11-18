<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { listKnowledgeFiles } from '@/api/knowledge-base'
import { formatVideoDuration, type VideoListInfo } from '@/api/knowledge-base/video'
import VideoThumbnail from './VideoThumbnail.vue'

const props = defineProps<{ knowledgeBaseId: string }>()
const emit = defineEmits<{ (e: 'open-source-doc', knowledgeId: string): void }>()
const { t } = useI18n()
type VideoItem = { id: string; file_name?: string; title?: string; parse_status?: string; video?: VideoListInfo }
const items = ref<VideoItem[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const error = ref(false)
const pageSize = 24
let generation = 0

async function load() {
  const current = ++generation
  loading.value = true
  error.value = false
  try {
    const response = await listKnowledgeFiles(props.knowledgeBaseId, {
      page: page.value, page_size: pageSize, file_type: 'mp4', sort_by: 'created_at', sort_order: 'desc',
    }) as { data?: VideoItem[]; total?: number }
    if (current !== generation) return
    items.value = response.data || []
    total.value = response.total || 0
  } catch {
    if (current === generation) error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

function fileName(item: VideoItem): string {
  const name = item.file_name || item.title || item.id
  return /\.mp4$/i.test(name) ? name : `${name}.mp4`
}

function changePage(next: number) {
  page.value = next
  void load()
}

watch(() => props.knowledgeBaseId, () => { page.value = 1; void load() })
onMounted(load)
</script>

<template>
  <section class="video-gallery">
    <header class="video-gallery__header">
      <h3>{{ t('videoKnowledge.tab') }} <span v-if="total">{{ total }}</span></h3>
      <t-button variant="text" shape="square" :title="t('videoKnowledge.refresh')" :aria-label="t('videoKnowledge.refresh')" @click="load">
        <template #icon><t-icon name="refresh" /></template>
      </t-button>
    </header>
    <div v-if="loading" class="video-gallery__state"><t-loading size="small" /></div>
    <div v-else-if="error" class="video-gallery__state">{{ t('videoKnowledge.loadFailed') }}</div>
    <div v-else-if="!items.length" class="video-gallery__state">{{ t('videoKnowledge.empty') }}</div>
    <div v-else class="video-gallery__grid">
      <button v-for="item in items" :key="item.id" type="button" class="video-gallery__item" @click="emit('open-source-doc', item.id)">
        <div class="video-gallery__poster">
          <VideoThumbnail v-if="item.video?.thumbnail_id" :knowledge-id="item.id" :frame-id="item.video.thumbnail_id" />
          <t-icon v-else name="video" size="32px" />
          <span v-if="item.video?.duration_ms" class="video-gallery__duration">{{ formatVideoDuration(item.video.duration_ms) }}</span>
        </div>
        <span class="video-gallery__name" :title="fileName(item)">{{ fileName(item) }}</span>
        <span class="video-gallery__status">{{ item.video?.status || item.parse_status || '' }}</span>
      </button>
    </div>
    <div v-if="total > pageSize" class="video-gallery__pager">
      <t-button variant="outline" shape="square" :disabled="page <= 1" :title="t('videoKnowledge.previousPage')" @click="changePage(page - 1)">
        <template #icon><t-icon name="chevron-left" /></template>
      </t-button>
      <span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
      <t-button variant="outline" shape="square" :disabled="page * pageSize >= total" :title="t('videoKnowledge.nextPage')" @click="changePage(page + 1)">
        <template #icon><t-icon name="chevron-right" /></template>
      </t-button>
    </div>
  </section>
</template>

<style scoped>
.video-gallery { width: 100%; min-width: 0; padding: 20px 28px 36px; box-sizing: border-box; }
.video-gallery__header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 18px; }
.video-gallery__header h3 { margin: 0; font-size: 16px; font-weight: 600; }
.video-gallery__header span { color: var(--td-text-color-secondary); font-size: 13px; margin-left: 6px; }
.video-gallery__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 16px; }
.video-gallery__item { min-width: 0; padding: 0; text-align: left; border: 1px solid var(--td-component-stroke); border-radius: 6px; background: var(--td-bg-color-container); cursor: pointer; overflow: hidden; }
.video-gallery__item:hover { border-color: var(--td-brand-color); }
.video-gallery__poster { height: 128px; position: relative; display: flex; align-items: center; justify-content: center; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-placeholder); }
.video-gallery__duration { position: absolute; right: 7px; bottom: 7px; padding: 2px 5px; border-radius: 3px; background: rgba(0, 0, 0, .76); color: white; font-size: 12px; }
.video-gallery__name, .video-gallery__status { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 0 10px; }
.video-gallery__name { margin-top: 10px; font-size: 13px; color: var(--td-text-color-primary); }
.video-gallery__status { margin: 5px 0 10px; font-size: 12px; color: var(--td-text-color-secondary); }
.video-gallery__state { min-height: 180px; display: flex; align-items: center; justify-content: center; color: var(--td-text-color-secondary); }
.video-gallery__pager { display: flex; align-items: center; justify-content: flex-end; gap: 10px; margin-top: 20px; }
@media (max-width: 600px) { .video-gallery { padding: 16px; } .video-gallery__grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; } .video-gallery__poster { height: 100px; } }
</style>
