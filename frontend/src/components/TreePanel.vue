<script setup>
// Recursive namespace/key tree — the Go-rewrite counterpart of
// BetterTreeView + namespaceitem. Clicking a key emits "open".
import { ref, computed } from 'vue'

const props = defineProps({
  node: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})
const emit = defineEmits(['open'])

const open = ref(props.depth === 0 || false)

const typeColor = {
  string: '#4a90d9',
  hash: '#4aa87c',
  list: '#c98a3d',
  set: '#9a6fc0',
  zset: '#c95d7c',
  stream: '#3d9db3',
  ReJSON: '#d4a017',
}

const badge = computed(() => (props.node.isNamespace ? null : props.node.keyType || '?'))
</script>

<template>
  <div class="node" :style="{ paddingLeft: depth * 14 + 'px' }">
    <div class="row" @click="node.isNamespace && (open = !open)">
      <span class="caret" v-if="node.isNamespace">{{ open ? '▾' : '▸' }}</span>
      <span class="spacer" v-else></span>
      <span :class="node.isNamespace ? 'ns' : 'key clickable'"
            @click="!node.isNamespace && emit('open', node)">{{ node.name }}</span>
      <span class="count" v-if="node.isNamespace">{{ node.count }}</span>
      <span class="type-badge" v-else-if="badge" :style="{ background: typeColor[badge] || '#888' }">
        {{ badge }}
      </span>
    </div>
    <div v-if="node.isNamespace && open">
      <TreePanel v-for="child in node.children || []" :key="child.fullPath + child.name"
                 :node="child" :depth="depth + 1" @open="n => emit('open', n)" />
    </div>
  </div>
</template>

<style scoped>
.row { display: flex; align-items: center; gap: 6px; padding: 2px 6px; border-radius: 5px; }
.node .row:hover { background: rgba(128, 128, 128, 0.12); }
.caret { width: 14px; cursor: pointer; color: #999; user-select: none; }
.spacer { width: 14px; }
.ns { font-weight: 600; cursor: pointer; }
.key { font-family: 'SF Mono', Menlo, monospace; font-size: 12.5px; word-break: break-all; }
.clickable { cursor: pointer; }
.clickable:hover { text-decoration: underline; }
.count { color: #999; font-size: 11px; }
.type-badge { font-size: 10px; color: #fff; padding: 1px 6px; border-radius: 8px; text-transform: uppercase; }
</style>
