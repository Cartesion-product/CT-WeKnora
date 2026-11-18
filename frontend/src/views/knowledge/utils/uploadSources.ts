import { kbFileTypeVerification } from '@/utils'

export const UPLOAD_VIDEO_EXTENSIONS = ['mov', 'avi', 'mkv', 'webm', 'wmv', 'flv']

export function getUploadFileExt(file: File): string {
  const dot = file.name.lastIndexOf('.')
  if (dot < 0) return ''
  return file.name.substring(dot + 1).toLowerCase()
}

export function getUploadFileKey(file: File): string {
  const path = (file as File & { webkitRelativePath?: string }).webkitRelativePath || ''
  return `${path || file.name}\0${file.size}`
}

export interface FilterUploadFilesOptions {
  supportedFileTypes?: Set<string> | string[]
  fromFolder?: boolean
  multiFile?: boolean
}

export interface FilterUploadFilesResult {
  validFiles: File[]
  skippedCount: number
  videoFilteredCount: number
  videoDisabledCount: number
  hiddenFileCount: number
}

export function filterUploadFiles(
  files: FileList | File[],
  options: FilterUploadFilesOptions = {},
): FilterUploadFilesResult {
  const list = Array.from(files)
  const dynamicTypesRaw = options.supportedFileTypes
    ? options.supportedFileTypes instanceof Set
      ? options.supportedFileTypes
      : new Set(options.supportedFileTypes)
    : undefined
  // An empty set means the parser-engine list hasn't loaded yet (race with the
  // async fetch on mount). Treat it as "unknown" and fall back to the default
  // whitelist instead of rejecting every file as unsupported.
  const dynamicTypes = dynamicTypesRaw && dynamicTypesRaw.size > 0 ? dynamicTypesRaw : undefined

  const validFiles: File[] = []
  let skippedCount = 0
  let videoFilteredCount = 0
  let videoDisabledCount = 0
  let hiddenFileCount = 0
  const multiFile = options.multiFile ?? list.length > 1
  const videoEnabled = (window as Window & { __RUNTIME_CONFIG__?: { VIDEO_INGEST_ENABLED?: boolean } })
    .__RUNTIME_CONFIG__?.VIDEO_INGEST_ENABLED === true || import.meta.env.VITE_VIDEO_INGEST_ENABLED === '1'

  for (const file of list) {
    if (options.fromFolder) {
      const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name
      if (relativePath.split('/').some(part => part.startsWith('.'))) {
        hiddenFileCount++
        continue
      }
    }

    const fileExt = getUploadFileExt(file)
    if (UPLOAD_VIDEO_EXTENSIONS.includes(fileExt)) {
      videoFilteredCount++
      continue
    }

    if (fileExt === 'mp4') {
      if (videoEnabled) validFiles.push(file)
      else videoDisabledCount++
      continue
    }
    if (kbFileTypeVerification(file, multiFile, dynamicTypes)) {
      skippedCount++
      continue
    }

    validFiles.push(file)
  }

  return { validFiles, skippedCount, videoFilteredCount, videoDisabledCount, hiddenFileCount }
}
