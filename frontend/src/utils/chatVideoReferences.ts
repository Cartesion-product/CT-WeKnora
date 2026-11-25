import type { KnowledgeReferenceLike } from './referenceSources'

export type ChatVideoReference = {
  key: string
  knowledgeId: string
  name: string
  snippet: string
  startMs?: number
  segmentId?: string
  processingRunId?: string
}

/** Retrieval order is already relevance order. Show each video once, at its first cited segment. */
export function selectChatVideoReferences(references: KnowledgeReferenceLike[], limit = 3): ChatVideoReference[] {
  const videos = new Map<string, ChatVideoReference>()
  for (const reference of references) {
    const media = reference.media
    const id = reference.knowledge_id
    if (!id || media?.type !== 'video') continue

    const position = media.start_ms ?? media.frame_ms
    const existing = videos.get(id)
    if (existing) {
      // A summary citation may precede the segment that has a playable timestamp.
      if (existing.startMs === undefined && position !== undefined) {
        existing.startMs = position
        existing.segmentId = media.segment_id
        existing.processingRunId = media.processing_run_id
        existing.snippet = String(reference.content || '').replace(/\s+/g, ' ').trim().slice(0, 140)
      }
      continue
    }
    if (videos.size >= limit) continue
    videos.set(id, {
      key: id,
      knowledgeId: id,
      name: reference.knowledge_filename || reference.knowledge_title || id,
      snippet: String(reference.content || '').replace(/\s+/g, ' ').trim().slice(0, 140),
      startMs: position,
      segmentId: media.segment_id,
      processingRunId: media.processing_run_id,
    })
  }
  return [...videos.values()]
}
