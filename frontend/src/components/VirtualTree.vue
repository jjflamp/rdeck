<script setup>
// Virtualized key tree: flattens the namespace tree into visible rows and
// renders only the scroll window — supports very large keyspaces
// (plan performance acceptance: 1M-key trees must stay responsive).
import { ref, computed } from 'vue'

const props = defineProps({
  nodes: { type: Array, required: true }, // top-level tree nodes
})
const emit = defineEmits(['open'])

const ROW_H = 26
const OVERSCAN = 10

const expanded = ref({}) // fullPath -> bool
const scrollTop = ref(0)
const viewportH = ref(480)

const typeColor = {
  string: '#4a90d9', hash: '#4aa87c', list: '#c98a3d', set: '#9a6fc0',
  zset: '#c95d7c', stream: '#3d9db3', ReJSON: '#d4a017',
}

function toggle(fullPath) {
  expanded.value = { ...expanded.value, [fullPath]: !expanded.value[fullPath] }
}

// Flatten visible rows following expanded state.
const rows = computed(() => {
  const out = []
  const walk = (list, depth) => {
    if (!list) return
    for (const n of list) {
      out.push({ node: n, depth })
      if (n.isNamespace && expanded.value[n.fullPath]) {
        walk(n.children || [], depth + 1)
      }
    }
  }
  walk(props.nodes, 0)
  return out
})

const total = computed(() => rows.value.length)
const startIdx = computed(() => Math.max(0, Math.floor(scrollTop.value / ROW_H) - OVERSCAN))
const endIdx = computed(() =>
  Math.min(total.value, Math.ceil((scrollTop.value + viewportH.value) / ROW_H) + OVERSCAN))
const visible = computed(() => rows.value.slice(startIdx.value, endIdx.value))
const padTop = computed(() => startIdx.value * ROW_H)
const padBottom = computed(() => Math.max(0, (total.value - endIdx.value) * ROW_H))

function onScroll(e) {
  scrollTop.value = e.target.scrollTop
  viewportH.value = e.target.clientHeight
}

function onRowClick(row) {
  if (row.node.isNamespace) toggle(row.node.fullPath)
  else emit('open', row.node)
}
</script>

<template>
  <div class="vtree" @scroll="onScroll">
    <div :style="{ height: padTop + 'px' }"></div>
    <div v-for="row in visible" :key="row.node.fullPath + row.node.name"
         class="vrow" :style="{ paddingLeft: row.depth * 14 + 'px' }"
         @click="onRowClick(row)">
      <span class="caret" v-if="row.node.isNamespace">{{ expanded[row.node.fullPath] ? '▾' : '▸' }}</span>
      <span class="spacer" v-else></span>
      <span :class="row.node.isNamespace ? 'ns' : 'key'">{{ row.node.name }}</span>
      <span class="count" v-if="row.node.isNamespace">{{ row.node.count }}</span>
      <span class="type-badge" v-else-if="row.node.keyType"
            :style="{ background: typeColor[row.node.keyType] || '#888' }">{{ row.node.keyType }}</span>
    </div>
    <div :style="{ height: padBottom + 'px' }"></div>
  </div>
</template>

<style scoped>
.vtree { flex: 1; overflow: auto; contain: strict; }
.vrow { display: flex; align-items: center; gap: 6px; height: 26px; padding-right: 8px; border-radius: 5px; cursor: default; }
.vrow:hover { background: rgba(128,128,128,.12); }
.caret { width: 14px; color: #999; user-select: none; }
.spacer { width: 14px; }
.ns { font-weight: 600; cursor: pointer; }
.key { font-family: 'SF Mono', Menlo, monospace; font-size: 12.5px; word-break: break-all; cursor: pointer; }
.key:hover { text-decoration: underline; }
.count { color: #999; font-size: 11px; }
.type-badge { font-size: 10px; color: #fff; padding: 1px 6px; border-radius: 8px; text-transform: uppercase; }
</style>
