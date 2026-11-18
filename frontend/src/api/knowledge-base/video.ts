import { del, get, post, put } from '@/utils/request'
import { getApiBaseUrl } from '@/utils/api-base'
import type { KnowledgeProcessOverrides } from '@/types/knowledgeProcess'

export interface VideoUploadSession {
  upload_id: string
  part_size: number
  completed_parts: number[]
  status: string
  knowledge_id?: string
}

export interface VideoEvidence {
  evidence_id: string
  segment_id: string
  modality: string
  text?: string
  content?: string
  start_ms?: number
  end_ms?: number
  frame_ms?: number
}

export interface VideoSegment {
  segment_id: string
  start_ms: number
  end_ms: number
  content?: string
  text?: string
  chunk_id?: string
}

export interface VideoKeyframe {
  id: string
  segment_id?: string
  frame_ms: number
  thumbnail_url?: string
}

export interface VideoDetail {
  asset: { status?: string; duration_ms?: number; failure_code?: string; active_run_id?: string; current_run_id?: string } | null
  run: { id?: string; status?: string; stage?: string; failure_code?: string; completed_windows?: number; total_windows?: number } | null
  segments: VideoSegment[]
  evidences: VideoEvidence[]
  keyframes: VideoKeyframe[]
  next_offset?: number | null
}

export interface VideoListInfo {
  duration_ms: number
  status: string
  stage?: string
  thumbnail_id?: string
}

export function formatVideoDuration(ms?: number): string {
  if (!ms || ms < 0) return ''
  const totalSeconds = Math.floor(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  return hours
    ? `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
    : `${minutes}:${String(seconds).padStart(2, '0')}`
}

type Envelope<T> = { success: boolean; data: T }

const uploadPath = (kbId: string, uploadId?: string) =>
  `/api/v1/knowledge-bases/${encodeURIComponent(kbId)}/knowledge/video-uploads${uploadId ? `/${encodeURIComponent(uploadId)}` : ''}`

export async function createVideoUpload(kbId: string, input: {
  file_name: string
  size: number
  mime: string
  folder_path?: string
  tag_ids?: string[]
  process_config?: KnowledgeProcessOverrides
}): Promise<VideoUploadSession> {
  const result = await post<Envelope<VideoUploadSession>>(uploadPath(kbId), input)
  return result.data
}

export async function getVideoUpload(kbId: string, uploadId: string): Promise<VideoUploadSession> {
  const result = await get<Envelope<VideoUploadSession>>(uploadPath(kbId, uploadId))
  return result.data
}

export async function cancelVideoUpload(kbId: string, uploadId: string): Promise<void> {
  await del(uploadPath(kbId, uploadId))
}

export async function uploadVideoPart(
  kbId: string, uploadId: string, partNumber: number, bytes: Blob,
  onProgress: (loaded: number) => void, signal: AbortSignal,
): Promise<void> {
  await put(`${uploadPath(kbId, uploadId)}/parts/${partNumber}`, bytes, {
    signal,
    timeout: 0,
    headers: { 'Content-Type': 'application/octet-stream' },
    onUploadProgress: (event: { loaded?: number }) => onProgress(event.loaded ?? 0),
  })
}

export async function completeVideoUpload(kbId: string, uploadId: string): Promise<VideoUploadSession> {
  const result = await post<Envelope<VideoUploadSession>>(`${uploadPath(kbId, uploadId)}/complete`)
  return result.data
}

export async function getVideoDetail(knowledgeId: string, options: { offset?: number; segmentId?: string } = {}): Promise<VideoDetail> {
  const query = new URLSearchParams({ limit: '100' })
  if (options.offset) query.set('offset', String(options.offset))
  if (options.segmentId) query.set('segment_id', options.segmentId)
  const result = await get<Envelope<VideoDetail>>(`/api/v1/knowledge/${encodeURIComponent(knowledgeId)}/video?${query}`)
  return result.data
}

export async function getVideoStatus(knowledgeId: string) {
  const result = await get<Envelope<{ upload_status: string; process_status: string; asset: VideoDetail['asset']; run: VideoDetail['run'] }>>(
    `/api/v1/knowledge/${encodeURIComponent(knowledgeId)}/media-status`)
  return result.data
}

export async function getVideoPlaybackUrl(knowledgeId: string): Promise<{ url: string; token: string; expiresAt: number }> {
  const path = `/api/v1/knowledge/${encodeURIComponent(knowledgeId)}/video`
  const result = await post<Envelope<{ token: string; expires_at: string | number }>>(`${path}/playback-token`)
  const base = getApiBaseUrl()
  const url = `${base}${path}/stream?token=${encodeURIComponent(result.data.token)}`
  const expiry = result.data.expires_at
  const expiresAt = typeof expiry === 'number' ? (expiry < 1e12 ? expiry * 1000 : expiry) : Date.parse(expiry)
  return { url, token: result.data.token, expiresAt }
}

export function getVideoKeyframeUrl(knowledgeId: string, frameId: string, token: string): string {
  return `${getApiBaseUrl()}/api/v1/knowledge/${encodeURIComponent(knowledgeId)}/video/keyframes/${encodeURIComponent(frameId)}?token=${encodeURIComponent(token)}`
}
