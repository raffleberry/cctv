// Typed client for the Go backend (plain REST over fetch).

export interface Camera {
  name: string
  color?: string
  rtsp_url?: string
  retention_days?: number
  rtsp_transport?: string
}

export interface BlockPresence {
  block: string // "00-06"
  present: boolean
  ts_count: number
  playlists: string[] // video URLs relative to server root
}

export interface PlaylistInfo {
  name: string
  url: string // relative URL to .m3u8
  start: string // RFC3339
  duration_sec: number
}

export interface Timeline {
  camera: string
  date: string
  block: string
  block_start: string // RFC3339
  slot_sec: number // granularity, 10
  slots: number // e.g. 2160 for a 6h block
  coverage: number[] // present slot indexes
  playlists: PlaylistInfo[]
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`${url}: ${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

export const api = {
  cameras: () => getJSON<Camera[]>('/api/cameras'),
  dates: (cams: string[]) =>
    getJSON<Record<string, string[]>>(`/api/dates?cams=${encodeURIComponent(cams.join(','))}`),
  blocks: (date: string, cams: string[]) =>
    getJSON<Record<string, BlockPresence[]>>(
      `/api/blocks?date=${encodeURIComponent(date)}&cams=${encodeURIComponent(cams.join(','))}`,
    ),
  timeline: (date: string, block: string, cam: string) =>
    getJSON<Timeline>(
      `/api/timeline?date=${encodeURIComponent(date)}&block=${encodeURIComponent(block)}&cam=${encodeURIComponent(cam)}`,
    ),
  liveStart: (cam: string) =>
    fetch(`/api/live/${encodeURIComponent(cam)}/start`, { method: 'POST' }).then((r) => {
      if (!r.ok) throw new Error(`live start ${cam}: ${r.status}`)
      return r.json()
    }),
};

export function blockLabel(b: string): string {
  return `${b.slice(0, 2)}:00 – ${b.slice(3)}:00`
}

/** Grid columns for N tiles: 1->1, 2->2, 3-4->2, 5-9->3, else 4. */
export function gridCols(n: number): string {
  if (n <= 1) return 'grid-cols-1'
  if (n <= 4) return 'grid-cols-2'
  if (n <= 9) return 'grid-cols-3'
  return 'grid-cols-4'
}
