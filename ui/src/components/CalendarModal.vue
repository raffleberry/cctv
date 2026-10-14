<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = defineProps<{
  open: boolean
  presentDates: Set<string> // "YYYY-MM-DD"
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'pick', date: string): void
}>()

const today = new Date()
const cursor = ref(new Date(today.getFullYear(), today.getMonth(), 1))

const monthLabel = computed(() =>
  cursor.value.toLocaleString(undefined, { month: 'long', year: 'numeric' }),
)

const cells = computed(() => {
  const y = cursor.value.getFullYear()
  const m = cursor.value.getMonth()
  const first = new Date(y, m, 1)
  const startDay = (first.getDay() + 6) % 7 // Monday-first
  const daysInMonth = new Date(y, m + 1, 0).getDate()
  const out: { date: Date | null; key: string }[] = []
  for (let i = 0; i < startDay; i++) out.push({ date: null, key: `pad-${i}` })
  for (let d = 1; d <= daysInMonth; d++) {
    const dt = new Date(y, m, d)
    out.push({ date: dt, key: iso(dt) })
  }
  return out
})

function iso(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

function step(dir: number) {
  cursor.value = new Date(cursor.value.getFullYear(), cursor.value.getMonth() + dir, 1)
}

watch(
  () => props.open,
  (o) => {
    if (o) cursor.value = new Date(today.getFullYear(), today.getMonth(), 1)
  },
)
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4">
    <div class="flex max-h-full w-full max-w-3xl flex-col overflow-hidden rounded-xl bg-neutral-900 text-white">
      <div class="flex items-center justify-between border-b border-white/10 px-5 py-3">
        <h2 class="text-lg font-semibold">Select date</h2>
        <button class="rounded bg-white/10 px-3 py-1 text-sm hover:bg-white/20" @click="emit('close')">
          Back
        </button>
      </div>
      <div class="flex items-center justify-between px-5 py-3">
        <button class="rounded bg-white/10 px-3 py-1 hover:bg-white/20" @click="step(-1)">‹</button>
        <div class="font-medium">{{ monthLabel }}</div>
        <button class="rounded bg-white/10 px-3 py-1 hover:bg-white/20" @click="step(1)">›</button>
      </div>
      <div class="grid grid-cols-7 gap-1 px-5 pb-2 text-center text-xs uppercase text-white/50">
        <div v-for="d in ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su']" :key="d">{{ d }}</div>
      </div>
      <div class="grid grid-cols-7 gap-1 overflow-y-auto px-5 pb-5">
        <button
          v-for="c in cells"
          :key="c.key"
          :disabled="!c.date || !presentDates.has(c.key)"
          @click="c.date && emit('pick', c.key)"
          :class="[
            'aspect-square rounded-lg text-sm',
            !c.date
              ? 'invisible'
              : presentDates.has(c.key)
                ? 'bg-emerald-600 font-semibold hover:bg-emerald-500'
                : 'bg-white/5 text-white/30',
          ]"
          :title="c.date ? (presentDates.has(c.key) ? `Footage: ${c.key}` : `No footage: ${c.key}`) : ''"
        >
          {{ c.date ? c.date.getDate() : '' }}
        </button>
      </div>
      <div class="border-t border-white/10 px-5 py-2 text-xs text-white/50">
        Highlighted dates have footage for the selected cameras.
      </div>
    </div>
  </div>
</template>
