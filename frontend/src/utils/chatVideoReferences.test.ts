import assert from 'node:assert/strict'
import { test } from 'node:test'
import { selectChatVideoReferences } from './chatVideoReferences'
import type { KnowledgeReferenceLike } from './referenceSources'

test('shows each referenced video once and keeps the first timed segment', () => {
  const refs: KnowledgeReferenceLike[] = [
    { knowledge_id: 'a', knowledge_filename: 'A.mp4', media: { type: 'video' }, content: 'summary' },
    { knowledge_id: 'a', media: { type: 'video', segment_id: 'a-1', start_ms: 31_000 }, content: 'first scene' },
    { knowledge_id: 'a', media: { type: 'video', segment_id: 'a-2', start_ms: 82_000 }, content: 'later scene' },
    { knowledge_id: 'b', knowledge_filename: 'B.mp4', media: { type: 'video', start_ms: 0 } },
    { knowledge_id: 'c', knowledge_filename: 'C.mp4', media: { type: 'video', start_ms: 5_000 } },
    { knowledge_id: 'd', knowledge_filename: 'D.mp4', media: { type: 'video' } },
    { knowledge_id: 'not-video', media: undefined },
  ]
  const videos = selectChatVideoReferences(refs)
  assert.deepEqual(videos.map(video => video.knowledgeId), ['a', 'b', 'c'])
  assert.equal(videos[0].startMs, 31_000)
  assert.equal(videos[0].segmentId, 'a-1')
  assert.equal(videos[0].snippet, 'first scene')
  assert.equal(videos[1].startMs, 0)
})
