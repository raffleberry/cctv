<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Hls from 'hls.js'

const props = defineProps<{
  src: string // hls url (live or recorded)
  label: string
  autoplay?: boolean
  muted?: boolean
}>()

const emit = defineEmits<{ (e: 'ready', el: HTMLVideoElement): void }>()

const videoEl = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null

function load(url: string) {
  const video = videoEl.value
  if (!video || !url) return
  destroyHls()
  if (url.endsWith('.m3u8') && Hls.isSupported()) {
    hls = new Hls({ enableWorker: true })
    hls.loadSource(url)
    hls.attachMedia(video)
  } else {
    video.src = url
  }
}

function destroyHls() {
  if (hls) {
    hls.destroy()
    hls = null
  }
}

/** Seek in seconds (used by review scrub). */
function seekTo(sec: number) {
  if (videoEl.value) videoEl.value.currentTime = sec
}

defineExpose({ seekTo })

onMounted(() => {
  load(props.src)
  if (videoEl.value) emit('ready', videoEl.value)
})

watch(
  () => props.src,
  (url) => load(url),
)

onBeforeUnmount(() => destroyHls())
</script>

<template>
  <div class="relative flex h-full w-full flex-col overflow-hidden rounded-lg bg-black">
    <video
      ref="videoEl"
      class="h-full w-full object-contain"
      controls
      playsinline
      :autoplay="autoplay ?? true"
      :muted="muted ?? true"
    />
    <div
      class="pointer-events-none absolute left-2 top-2 rounded bg-black/60 px-2 py-0.5 text-xs font-medium text-white"
    >
      {{ label }}
    </div>
  </div>
</template>
