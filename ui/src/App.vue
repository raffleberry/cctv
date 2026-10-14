<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import CameraTile from './components/CameraTile.vue'
import CalendarModal from './components/CalendarModal.vue'
import DayBlocks from './components/DayBlocks.vue'
import TimelineScrub from './components/TimelineScrub.vue'
import { api, gridCols, type BlockPresence, type Camera, type Timeline } from './api'

type Mode = 'live' | 'review'

const mode = ref<Mode>('live')
const cameras = ref<Camera[]>([])
const error = ref('')

// ---- live ----
const liveCams = ref<string[]>([])
const focused = ref<string | null>(null)
const liveDropdownOpen = ref(false)
const liveSrc = (cam: string) => `/live/${encodeURIComponent(cam)}/index.m3u8`

// ---- review ----
const reviewPool = ref<string[]>([]) // cameras considered (all by default)
const reviewDropdownOpen = ref(false)
const calOpen = ref(false)
const presentDates = ref<Set<string>>(new Set())
const date = ref<string | null>(null)
const blockPresence = ref<Record<string, BlockPresence[]>>({})
const block = ref<string | null>(null)
const reviewCams = ref<string[]>([])
const timelines = reactive<Record<string, Timeline>>({})
const position = ref(-1)
const videos = reactive<Record<string, HTMLVideoElement>>({})
const currentUrl = reactive<Record<string, string>>({})

const visibleLive = computed(() => (focused.value ? [focused.value] : liveCams.value))
const visibleReview = computed(() => reviewCams.value)

async function loadCameras() {
  try {
    cameras.value = await api.cameras()
    const names = cameras.value.map((c) => c.name)
    if (liveCams.value.length === 0) liveCams.value = names.slice(0, 4)
    if (reviewPool.value.length === 0) reviewPool.value = names.slice()
  } catch (e) {
    error.value = `Failed to load cameras: ${e}`
  }
}

/* ---------- live ---------- */
async function ensureLive(cam: string) {
  try {
    await api.liveStart(cam)
  } catch (e) {
    error.value = `Live start ${cam} failed: ${e}`
  }
}

watch(
  liveCams,
  (cams) => cams.forEach((c) => void ensureLive(c)),
  { immediate: false },
)

function toggleLiveCam(cam: string) {
  const i = liveCams.value.indexOf(cam)
  if (i >= 0) liveCams.value.splice(i, 1)
  else {
    liveCams.value.push(cam)
    void ensureLive(cam)
  }
}

/* ---------- review ---------- */
async function refreshDates() {
  if (reviewPool.value.length === 0) {
    presentDates.value = new Set()
    return
  }
  try {
    const m = await api.dates(reviewPool.value)
    const union = new Set<string>()
    Object.values(m).forEach((ds) => ds.forEach((d) => union.add(d)))
    presentDates.value = union
  } catch (e) {
    error.value = `Failed to load dates: ${e}`
  }
}

async function pickDate(d: string) {
  date.value = d
  block.value = null
  reviewCams.value = []
  calOpen.value = false
  try {
    blockPresence.value = await api.blocks(d, reviewPool.value)
  } catch (e) {
    error.value = `Failed to load blocks: ${e}`
  }
}

function camsWithFootage(b: string): string[] {
  return Object.entries(blockPresence.value)
    .filter(([, bs]) => (bs || []).some((x) => x.block === b && x.present))
    .map(([cam]) => cam)
}

async function pickBlock(b: string) {
  block.value = b
  position.value = -1
  // default: all cameras that have footage in this block
  reviewCams.value = camsWithFootage(b)
  await loadTimelines()
}

async function loadTimelines() {
  if (!date.value || !block.value) return
  for (const cam of reviewCams.value) {
    if (timelines[cam]?.date === date.value && timelines[cam]?.block === block.value) continue
    try {
      timelines[cam] = await api.timeline(date.value, block.value, cam)
    } catch (e) {
      error.value = `Timeline ${cam} failed: ${e}`
    }
  }
}

watch(reviewCams, () => void loadTimelines())

function onVideoReady(cam: string, el: HTMLVideoElement) {
  videos[cam] = el
  el.addEventListener('timeupdate', () => onTimeUpdate(cam, el))
}

function playlistFor(tl: Timeline | undefined, slot: number): { url: string; offset: number } | null {
  if (!tl) return null
  const t = new Date(tl.block_start).getTime() + slot * tl.slot_sec * 1000
  for (const p of tl.playlists) {
    const s = new Date(p.start).getTime()
    const e = s + p.duration_sec * 1000
    if (t >= s && t < e) return { url: p.url, offset: (t - s) / 1000 }
  }
  return null
}

function seekSlot(slot: number) {
  if (!date.value || !block.value) return
  position.value = slot
  for (const cam of reviewCams.value) {
    const tl = timelines[cam]
    const hit = playlistFor(tl, slot)
    const video = videos[cam]
    if (!hit || !video) continue
    const want = hit.url
    if (currentUrl[cam] !== want) {
      currentUrl[cam] = want
      video.src = want
      const off = hit.offset
      video.onloadedmetadata = () => {
        video.currentTime = off
        void video.play().catch(() => {})
      }
    } else {
      video.currentTime = hit.offset
      void video.play().catch(() => {})
    }
  }
}

function onTimeUpdate(cam: string, el: HTMLVideoElement) {
  // keep scrub position in sync from the first reviewing camera
  if (reviewCams.value[0] !== cam) return
  const tl = timelines[cam]
  const url = currentUrl[cam]
  if (!tl || !url) return
  const p = tl.playlists.find((x) => x.url === url)
  if (!p) return
  const abs = new Date(p.start).getTime() + el.currentTime * 1000
  const slot = Math.floor((abs - new Date(tl.block_start).getTime()) / (tl.slot_sec * 1000))
  if (slot >= 0 && slot < tl.slots) position.value = slot
}

function reviewSrc(cam: string): string {
  // initial src: first playlist of that camera
  const tl = timelines[cam]
  if (!tl || tl.playlists.length === 0) return ''
  if (!currentUrl[cam]) currentUrl[cam] = tl.playlists[0].url
  return currentUrl[cam]
}

function toggleReviewPool(cam: string) {
  const i = reviewPool.value.indexOf(cam)
  if (i >= 0) reviewPool.value.splice(i, 1)
  else reviewPool.value.push(cam)
  void refreshDates()
}

function toggleReviewCam(cam: string) {
  const i = reviewCams.value.indexOf(cam)
  if (i >= 0) reviewCams.value.splice(i, 1)
  else {
    reviewCams.value.push(cam)
    void loadTimelines()
  }
}

watch(mode, (m) => {
  if (m === 'review') void refreshDates()
})

loadCameras().then(() => {
  liveCams.value.forEach((c) => void ensureLive(c))
  void refreshDates()
})
</script>

<template>
  <div class="min-h-screen bg-neutral-950 text-white">
    <header class="sticky top-0 z-40 flex flex-wrap items-center gap-2 border-b border-white/10 bg-neutral-950/90 px-4 py-2 backdrop-blur">
      <h1 class="mr-2 text-lg font-bold">CCTV</h1>
      <div class="flex overflow-hidden rounded-lg border border-white/15 text-sm">
        <button
          :class="['px-4 py-1.5', mode === 'live' ? 'bg-white text-black' : 'text-white/70 hover:bg-white/10']"
          @click="mode = 'live'"
        >
          Live
        </button>
        <button
          :class="['px-4 py-1.5', mode === 'review' ? 'bg-white text-black' : 'text-white/70 hover:bg-white/10']"
          @click="mode = 'review'"
        >
          Review
        </button>
      </div>

      <!-- live camera picker -->
      <div v-if="mode === 'live'" class="relative">
        <button
          class="rounded-lg bg-white/10 px-4 py-1.5 text-sm hover:bg-white/20"
          @click="liveDropdownOpen = !liveDropdownOpen"
        >
          Cameras ({{ liveCams.length }}) ▾
        </button>
        <div
          v-if="liveDropdownOpen"
          class="absolute left-0 z-50 mt-1 max-h-80 w-64 overflow-y-auto rounded-lg border border-white/15 bg-neutral-900 p-2 shadow-xl"
        >
          <label
            v-for="c in cameras"
            :key="c.name"
            class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-white/10"
          >
            <input type="checkbox" :checked="liveCams.includes(c.name)" @change="toggleLiveCam(c.name)" />
            <span class="inline-block h-3 w-3 rounded-full" :style="{ background: c.color || '#22c55e' }" />
            {{ c.name }}
          </label>
        </div>
      </div>

      <!-- review controls -->
      <template v-if="mode === 'review'">
        <button class="rounded-lg bg-white/10 px-4 py-1.5 text-sm hover:bg-white/20" @click="calOpen = true">
          📅 {{ date ?? 'Calendar' }}
        </button>
        <div class="relative">
          <button
            class="rounded-lg bg-white/10 px-4 py-1.5 text-sm hover:bg-white/20"
            @click="reviewDropdownOpen = !reviewDropdownOpen"
          >
            Cameras ({{ reviewCams.length }}) ▾
          </button>
          <div
            v-if="reviewDropdownOpen"
            class="absolute left-0 z-50 mt-1 max-h-80 w-72 overflow-y-auto rounded-lg border border-white/15 bg-neutral-900 p-2 shadow-xl"
          >
            <div class="px-2 py-1 text-[11px] uppercase text-white/40">pool (calendar scope)</div>
            <label
              v-for="c in cameras"
              :key="'p' + c.name"
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-white/10"
            >
              <input type="checkbox" :checked="reviewPool.includes(c.name)" @change="toggleReviewPool(c.name)" />
              {{ c.name }}
            </label>
            <template v-if="block">
              <div class="mt-2 border-t border-white/10 px-2 py-1 text-[11px] uppercase text-white/40">
                in view (footage for {{ block }})
              </div>
              <label
                v-for="c in cameras"
                :key="'v' + c.name"
                :class="[
                  'flex items-center gap-2 rounded px-2 py-1 text-sm',
                  camsWithFootage(block).includes(c.name) ? 'cursor-pointer hover:bg-white/10' : 'text-white/30',
                ]"
              >
                <input
                  type="checkbox"
                  :disabled="!camsWithFootage(block).includes(c.name)"
                  :checked="reviewCams.includes(c.name)"
                  @change="toggleReviewCam(c.name)"
                />
                {{ c.name }}
                <span v-if="!camsWithFootage(block).includes(c.name)" class="text-xs">(no footage)</span>
              </label>
            </template>
          </div>
        </div>
        <span v-if="block" class="text-sm text-white/60">{{ date }} · {{ block }}</span>
      </template>

      <button
        v-if="focused"
        class="ml-auto rounded-lg bg-white/10 px-4 py-1.5 text-sm hover:bg-white/20"
        @click="focused = null"
      >
        ← Back to grid
      </button>
    </header>

    <div v-if="error" class="mx-4 mt-2 rounded-lg bg-red-900/60 px-3 py-2 text-sm">{{ error }}</div>

    <!-- LIVE -->
    <main v-if="mode === 'live'" class="p-3">
      <div v-if="liveCams.length === 0" class="p-10 text-center text-white/50">
        Select cameras from the dropdown to start the multiview.
      </div>
      <div v-else :class="['grid gap-2', focused ? 'grid-cols-1' : gridCols(visibleLive.length)]">
        <div
          v-for="cam in visibleLive"
          :key="cam"
          :class="focused ? 'h-[calc(100vh-120px)]' : 'aspect-video'"
          @click="focused ? null : (focused = cam)"
        >
          <CameraTile :src="liveSrc(cam)" :label="`${cam} · LIVE`" :class="focused ? '' : 'cursor-pointer'" />
          <div v-if="!focused" class="mt-1 text-center text-xs text-white/40">click to fullscreen</div>
        </div>
      </div>
    </main>

    <!-- REVIEW -->
    <main v-if="mode === 'review'" class="space-y-3 p-3">
      <div v-if="!date" class="rounded-xl bg-neutral-900 p-10 text-center text-white/60">
        Open the calendar and pick a highlighted date with footage.
      </div>
      <template v-else>
        <DayBlocks
          :date="date"
          :presence="blockPresence"
          :cameras="reviewPool"
          @pick="pickBlock"
        />
        <div v-if="block && reviewCams.length === 0" class="rounded-xl bg-neutral-900 p-8 text-center text-white/50">
          No cameras selected — open the Cameras dropdown.
        </div>
        <div
          v-if="block && reviewCams.length > 0"
          :class="['grid gap-2', gridCols(visibleReview.length)]"
        >
          <div v-for="cam in visibleReview" :key="cam" class="aspect-video">
            <CameraTile
              :src="reviewSrc(cam)"
              :label="`${cam} · ${date} ${block}`"
              :autoplay="false"
              @ready="(el) => onVideoReady(cam, el)"
            />
          </div>
        </div>
        <div
          v-if="block && reviewCams.length > 0"
          class="space-y-2 rounded-xl bg-neutral-900 p-3"
        >
          <div v-for="cam in reviewCams" :key="'tl' + cam">
            <div class="mb-1 text-xs text-white/60">{{ cam }}</div>
            <TimelineScrub
              v-if="timelines[cam]"
              :slots="timelines[cam].slots"
              :coverage="timelines[cam].coverage"
              :position="position"
              :slot-sec="timelines[cam].slot_sec"
              :block-start-i-s-o="timelines[cam].block_start"
              @seek="seekSlot"
            />
            <div v-else class="text-xs text-white/30">loading…</div>
          </div>
        </div>
      </template>
    </main>

    <CalendarModal :open="calOpen" :present-dates="presentDates" @close="calOpen = false" @pick="pickDate" />
  </div>
</template>
