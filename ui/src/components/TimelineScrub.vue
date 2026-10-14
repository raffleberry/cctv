<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  slots: number
  coverage: number[] // sorted present slot indexes
  position: number // current slot index (-1 = none)
  slotSec?: number
  blockStartISO?: string
}>()

const emit = defineEmits<{ (e: 'seek', slot: number): void }>()

const present = computed(() => new Set(props.coverage))

function slotTime(slot: number): string {
  if (!props.blockStartISO) return `+${slot * (props.slotSec ?? 10)}s`
  const t = new Date(props.blockStartISO).getTime() + slot * (props.slotSec ?? 10) * 1000
  return new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function onClick(e: MouseEvent) {
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  const frac = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width))
  emit('seek', Math.floor(frac * props.slots))
}
</script>

<template>
  <div>
    <div
      class="flex h-10 w-full cursor-pointer overflow-hidden rounded border border-white/15"
      @click="onClick"
      :title="`click to seek (${slots} × ${slotSec ?? 10}s)`"
    >
      <div
        v-for="i in slots"
        :key="i"
        class="h-full"
        :style="{ width: `${100 / slots}%` }"
        :class="[
          present.has(i - 1) ? 'bg-emerald-500' : 'bg-white/10',
          i - 1 === position ? 'outline outline-1 outline-white' : '',
        ]"
        :title="present.has(i - 1) ? slotTime(i - 1) : `${slotTime(i - 1)} (missing)`"
      />
    </div>
    <div class="mt-1 flex justify-between text-[11px] text-white/50">
      <span>{{ slotTime(0) }}</span>
      <span v-if="position >= 0">{{ slotTime(position) }}</span>
      <span>{{ slotTime(slots - 1) }}</span>
    </div>
  </div>
</template>
