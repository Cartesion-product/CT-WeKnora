<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { getVideoKeyframeUrl, getVideoPlaybackUrl } from '@/api/knowledge-base/video'

const props = defineProps<{ knowledgeId: string; frameId: string }>()
const url = ref('')
let generation = 0

watch(() => [props.knowledgeId, props.frameId], async () => {
  const current = ++generation
  url.value = ''
  if (!props.knowledgeId || !props.frameId) return
  try {
    const grant = await getVideoPlaybackUrl(props.knowledgeId)
    if (generation === current) url.value = getVideoKeyframeUrl(props.knowledgeId, props.frameId, grant.token)
  } catch {
    // The file icon remains visible when a thumbnail is unavailable.
  }
}, { immediate: true })

onBeforeUnmount(() => { generation++ })
</script>

<template>
  <img v-if="url" class="video-thumbnail" :src="url" alt="" loading="lazy" @error="url = ''" />
</template>

<style scoped>
.video-thumbnail { width: 100%; height: 100%; object-fit: cover; border-radius: 3px; }
</style>
