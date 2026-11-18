import { defineStore } from 'pinia'
import { batchQueryKnowledge, uploadKnowledgeFile } from '@/api/knowledge-base/index'
import { useAuthStore } from './auth'
import type { KnowledgeStatusRow } from './uploadQueue'
import { createUploadTasks } from './uploadTasksCore'
import { createVideoUpload, getVideoUpload, uploadVideoPart, completeVideoUpload, cancelVideoUpload } from '@/api/knowledge-base/video'

export type { UploadBatch } from './uploadQueue'
export type { EnqueueUploadsInput } from './uploadTasksCore'

/**
 * Knowledge file uploads behind the floating progress panel
 * (components/upload-tasks/UploadTasksPanel.vue). Lives in a store rather than
 * the knowledge base page so uploads keep going, and stay visible, when the
 * user navigates elsewhere in the app. Behaviour is in uploadTasksCore.ts and
 * uploadQueue.ts; this file only connects it to the API and the page.
 */
export const useUploadTasksStore = defineStore('uploadTasks', () => {
  const auth = useAuthStore()

  return createUploadTasks({
    upload: async (batch, payload, onProgress, signal) => {
      if (payload.file.name.toLowerCase().endsWith('.mp4')) {
        const file = payload.file
        const qualifiedName = payload.fileName || file.name
        const slash = qualifiedName.lastIndexOf('/')
        const folderPath = slash < 0 ? '' : qualifiedName.slice(0, slash)
        const storageKey = `weknora-video-upload:${auth.user?.id || ''}:${auth.effectiveTenantId || ''}:${batch.kbId}:${qualifiedName}:${file.size}:${file.lastModified}`
        let uploadId = localStorage.getItem(storageKey)
        let session
        if (uploadId) {
          try { session = await getVideoUpload(batch.kbId, uploadId) }
          catch { localStorage.removeItem(storageKey); uploadId = null }
        }
        if (!session) {
          session = await createVideoUpload(batch.kbId, {
            file_name: file.name,
            size: file.size,
            mime: 'video/mp4',
            folder_path: folderPath || undefined,
            tag_ids: batch.tagIds,
            process_config: batch.processConfig,
          })
          uploadId = session.upload_id
          localStorage.setItem(storageKey, uploadId)
        }
        if (!uploadId) throw new Error('Video upload session was not created')
        if (session.knowledge_id) {
          localStorage.removeItem(storageKey)
          return { success: true, data: { id: session.knowledge_id, parse_status: 'pending' } }
        }
        const partSize = session.part_size
        if (!Number.isSafeInteger(partSize) || partSize <= 0) throw new Error('Invalid video upload part size')
        try {
          const completed = new Set(session.completed_parts || [])
          let sent = 0
          for (let part = 1; (part - 1) * partSize < file.size; part++) {
            const start = (part - 1) * partSize
            const end = Math.min(file.size, start + partSize)
            if (completed.has(part)) {
              sent += end - start
              onProgress(sent / file.size)
              continue
            }
            await uploadVideoPart(batch.kbId, uploadId, part, file.slice(start, end), bytes => {
              onProgress((sent + Math.min(end - start, bytes)) / file.size)
            }, signal)
            sent += end - start
            onProgress(sent / file.size)
          }
          if (signal.aborted) throw new DOMException('Upload cancelled', 'AbortError')
          const finished = await completeVideoUpload(batch.kbId, uploadId)
          if (!finished.knowledge_id) throw new Error('Video upload completed without a knowledge ID')
          localStorage.removeItem(storageKey)
          return { success: true, data: { id: finished.knowledge_id, parse_status: 'pending' } }
        } catch (error) {
          if (signal.aborted) {
            try { await cancelVideoUpload(batch.kbId, uploadId) } catch { /* The server also expires abandoned sessions. */ }
            localStorage.removeItem(storageKey)
          }
          throw error
        }
      }
      return uploadKnowledgeFile(
      batch.kbId,
      {
        file: payload.file,
        fileName: payload.fileName,
        tag_ids: batch.tagIds,
        process_config: batch.processConfig,
      },
      (event: { loaded?: number; total?: number; progress?: number }) => {
        onProgress(event.progress ?? (event.total ? (event.loaded ?? 0) / event.total : 0))
      },
      { signal },
      )
    },
    queryStatus: async (kbId, knowledgeIds) => {
      const query = knowledgeIds.map(id => `ids=${encodeURIComponent(id)}`).join('&')
      try {
        const result: any = await batchQueryKnowledge(query, kbId)
        return result?.success && Array.isArray(result.data) ? (result.data as KnowledgeStatusRow[]) : null
      } catch {
        return null
      }
    },
    emitListRefresh: (kbId, settled) => {
      window.dispatchEvent(new CustomEvent('knowledgeFileUploaded', { detail: { kbId, settled } }))
    },
    owner: [() => auth.user?.id, () => auth.effectiveTenantId],
  })
})
