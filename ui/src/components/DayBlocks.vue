<script setup lang="ts">
import type { BlockPresence } from '../api'
import { blockLabel } from '../api'

defineProps<{
  date: string
  // camera -> 4 blocks
  presence: Record<string, BlockPresence[]>
  cameras: string[]
}>()

const emit = defineEmits<{ (e: 'pick', block: string): void }>()

const BLOCKS = ['00-06', '06-12', '12-18', '18-24']

function anyPresent(block: string, presence: Record<string, BlockPresence[]>): boolean {
  return Object.values(presence)
    .flat()
    .some((b) => b.block === block && b.present)
}

function countFor(block: string, presence: Record<string, BlockPresence[]>): number {
  return Object.values(presence)
    .flat()
    .filter((b) => b.block === block && b.present).length
}
</script>

<template>
  <div class="rounded-xl bg-neutral-900 p-4 text-white">
    <h3 class="mb-1 text-sm font-semibold uppercase tracking-wide text-white/60">
      {{ date }} — pick a 6h block
    </h3>
    <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
      <button
        v-for="b in BLOCKS"
        :key="b"
        :disabled="!anyPresent(b, presence)"
        @click="emit('pick', b)"
        :class="[
          'rounded-lg border p-3 text-left',
          anyPresent(b, presence)
            ? 'border-emerald-500/50 bg-emerald-950 hover:bg-emerald-900'
            : 'border-white/10 bg-white/5 text-white/30',
        ]"
      >
        <div class="text-sm font-semibold">{{ blockLabel(b) }}</div>
        <div class="mt-1 text-xs opacity-70">
          {{ anyPresent(b, presence) ? `${countFor(b, presence)}/${cameras.length} cams` : 'no footage' }}
        </div>
        <div class="mt-2 flex h-2 gap-0.5 overflow-hidden rounded">
          <div
            v-for="cam in cameras"
            :key="cam"
            class="h-full flex-1"
            :class="
              (presence[cam] || []).some((x) => x.block === b && x.present)
                ? 'bg-emerald-500'
                : 'bg-white/10'
            "
            :title="cam"
          />
        </div>
      </button>
    </div>
  </div>
</template>
