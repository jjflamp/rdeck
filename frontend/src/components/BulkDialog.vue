<script setup>
// Bulk operations dialog — delete / copy / ttl / rdb_import with preview
// and progress events (counterpart of BulkOperationsDialog.qml).
import { ref, onUnmounted, computed } from 'vue'
import { EVENTS } from '../api/events'

const { EventsOn, EventsOff } = window.runtime

const props = defineProps({
  connID: { type: String, required: true },
  db: { type: Number, required: true },
  connections: { type: Array, required: true }, // all saved connections
})
const emit = defineEmits(['close', 'changed'])

const A = window.go.app.App

const op = ref('delete')
const pattern = ref('*')
const preview = ref(null)
const running = ref(false)
const progress = ref({ done: 0, total: 0, msg: '' })
const doneMsg = ref('')
const ttl = ref(3600)
const targetKey = ref('') // "connID:db"
const rdbPath = ref('')
const include = ref('')

const runID = ref(null)

const targetList = computed(() => {
  const out = []
  for (const c of props.connections) {
    for (let d = 0; d < 16; d++) out.push({ id: c.id, db: d, label: `${c.name || c.host} · db${d}` })
  }
  return out
})

async function doPreview() {
  doneMsg.value = ''
  preview.value = await A.BulkPreview(props.connID, props.db, pattern.value || '*')
}

async function run() {
  const [targetConn, targetDb] = (targetKey.value || ':').split(':')
  const req = {
    op: op.value, conn_id: props.connID, db: props.db, pattern: pattern.value || '*',
    target_conn_id: targetConn, target_db: Number(targetDb) || 0,
    ttl: Number(ttl.value) || 0,
    rdb_path: rdbPath.value, include: include.value ? include.value.split(',').map(s => s.trim()) : [],
  }
  running.value = true
  doneMsg.value = ''
  progress.value = { done: 0, total: preview.value?.total || 0, msg: '' }
  runID.value = await A.BulkRun(req)
}

function onProgress(id, done, total, msg) {
  if (id !== runID.value) return
  progress.value = { done, total, msg }
}
function onDone(id, summary) {
  if (id !== runID.value) return
  running.value = false
  doneMsg.value = summary.ok
    ? `完成：${JSON.stringify(summary)}`
    : `失败：${summary.error}`
  if (summary.ok) emit('changed')
}

onMounted(() => {
  EventsOn(EVENTS.BULK_PROGRESS, onProgress)
  EventsOn(EVENTS.BULK_DONE, onDone)
})
onUnmounted(() => {
  EventsOff(EVENTS.BULK_PROGRESS)
  EventsOff(EVENTS.BULK_DONE)
})

const pct = computed(() =>
  progress.value.total ? Math.round((progress.value.done / progress.value.total) * 100) : 0)
</script>

<template>
  <div class="modal-mask" @click.self="!running && emit('close')">
    <div class="modal">
      <h4>批量操作</h4>

      <label>操作类型
        <select v-model="op">
          <option value="delete">删除 keys</option>
          <option value="copy">复制 keys（跨连接/库）</option>
          <option value="ttl">批量设置 TTL</option>
          <option value="rdb_import">导入 RDB 文件</option>
        </select>
      </label>

      <template v-if="op !== 'rdb_import'">
        <label>key 模式
          <input v-model="pattern" placeholder="user:*" @keyup.enter="doPreview" />
        </label>
        <button v-if="!running" @click="doPreview">预览匹配 keys</button>
        <p v-if="preview" class="preview">
          匹配 {{ preview.total }} keys
          <span v-if="!preview.complete">（达到扫描上限）</span>
        </p>
      </template>

      <template v-if="!running">
        <label v-if="op === 'copy'">目标
          <select v-model="targetKey">
            <option value="" disabled>选择目标连接 · db</option>
            <option v-for="t in targetList" :key="t.id + t.db" :value="t.id + ':' + t.db">{{ t.label }}</option>
          </select>
        </label>
        <label v-if="op === 'ttl'">TTL 秒数（0 = 移除过期）
          <input v-model.number="ttl" type="number" />
        </label>
        <template v-if="op === 'rdb_import'">
          <label>RDB 文件路径<input v-model="rdbPath" placeholder="/path/to/dump.rdb" /></label>
          <label>key 过滤模式（逗号分隔，留空全部）<input v-model="include" placeholder="user:*, cache:*" /></label>
        </template>
      </template>

      <div class="progress-wrap" v-if="running">
        <div class="bar"><div class="fill" :style="{ width: pct + '%' }"></div></div>
        <span class="pct">{{ pct }}% {{ progress.msg }}</span>
      </div>
      <p v-if="doneMsg" class="done">{{ doneMsg }}</p>

      <div class="btn-row">
        <button v-if="!running" class="primary" :disabled="op !== 'rdb_import' && !preview"
                @click="run">执行</button>
        <button v-if="!running" @click="emit('close')">取消</button>
        <button v-else disabled>执行中…</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-mask { position: fixed; inset: 0; background: rgba(0,0,0,.4); display: flex; align-items: center; justify-content: center; z-index: 20; }
.modal { background: var(--panel); border-radius: 12px; padding: 22px; width: min(520px, 90vw); display: flex; flex-direction: column; gap: 12px; }
label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
input, select { padding: 7px 9px; border: 1px solid var(--border); border-radius: 6px; }
.preview { font-size: 13px; color: #3567b3; }
.btn-row { display: flex; gap: 10px; }
button { padding: 7px 14px; border: 1px solid var(--border); border-radius: 6px; background: transparent; cursor: pointer; }
button.primary { background: #d82c20; border-color: #d82c20; color: #fff; }
button:disabled { opacity: .5; cursor: not-allowed; }
.progress-wrap { display: flex; align-items: center; gap: 10px; }
.bar { flex: 1; height: 8px; background: rgba(128,128,128,.2); border-radius: 4px; overflow: hidden; }
.fill { height: 100%; background: #3567b3; transition: width .2s; }
.pct { font-size: 12px; color: var(--muted); min-width: 90px; }
.done { font-size: 13px; color: #2a7; word-break: break-all; }
</style>
