<script setup>
// Server panel — INFO cards + SVG charts, SlowLog, Clients, PubSub.
// (Counterpart of ServerActionTabs/ServerCharts/ServerSlowlog/ServerPubSub.)
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { EVENTS } from '../api/events'

const { EventsOn, EventsOff } = window.runtime

const props = defineProps({
  connID: { type: String, required: true },
  db: { type: Number, required: true },
  connCfg: { type: Object, required: false, default: null },
  retryConn: { type: Function, required: false, default: null },
})

const A = window.go.app.App
const cid = () => props.connCfg?.id || props.connID

const tab = ref('info') // info | slowlog | clients | pubsub
const info = ref(null)
const infoErr = ref('')
const history = ref([]) // {ts, usedMemory, opsPerSec, connectedClients}
let pollTimer = null

const slowlog = ref([])
const clients = ref([])

const pubChannel = ref('')
const pubSession = ref('')
const pubMessages = ref([])

const memSeries = computed(() => history.value.map(h => h.usedMemory))
const opsSeries = computed(() => history.value.map(h => h.opsPerSec))

async function poll() {
  try {
    info.value = await A.GetServerInfo(cid(), props.db)
    infoErr.value = ''
    const s = info.value.sections
    const mem = parseMem(s['Memory']?.used_memory_human)
    history.value.push({
      ts: Date.now(),
      usedMemory: mem,
      opsPerSec: Number(s['Stats']?.instantaneous_ops_per_sec || 0),
      connectedClients: Number(s['Clients']?.connected_clients || 0),
    })
    if (history.value.length > 60) history.value.shift()
  } catch (e) {
    // Self-heal once when the manager lost the connection.
    if (String(e).includes('is not open') && props.retryConn) {
      try { await props.retryConn(); infoErr.value = ''; return } catch { /* fall through */ }
    }
    infoErr.value = String(e)
  }
}

function parseMem(h) {
  if (!h) return 0
  const m = String(h).match(/^([\d.]+)([KMG]?)/)
  if (!m) return 0
  const mult = { '': 1, K: 1024, M: 1024 ** 2, G: 1024 ** 3 }[m[2]]
  return parseFloat(m[1]) * mult
}

function fmtMem(b) {
  if (b > 1024 ** 3) return (b / 1024 ** 3).toFixed(2) + 'G'
  if (b > 1024 ** 2) return (b / 1024 ** 2).toFixed(2) + 'M'
  if (b > 1024) return (b / 1024).toFixed(1) + 'K'
  return b + 'B'
}

const cards = computed(() => {
  const s = info.value?.sections || {}
  return [
    { label: 'redis', v: s['Server']?.redis_version || '-' },
    { label: 'mode', v: s['Server']?.redis_mode || 'standalone' },
    { label: 'clients', v: s['Clients']?.connected_clients || '0' },
    { label: 'memory', v: s['Memory']?.used_memory_human || '-' },
    { label: 'ops/sec', v: s['Stats']?.instantaneous_ops_per_sec || '0' },
    { label: 'hit rate', v: hitRate.value },
    { label: 'uptime', v: (Number(s['Server']?.uptime_in_days) || 0) + ' 天' },
    { label: 'keys', v: Object.keys(s['Keyspace'] || {}).length + ' dbs' },
  ]
})

const hitRate = computed(() => {
  const s = info.value?.sections?.['Stats'] || {}
  const hits = Number(s.keyspace_hits || 0), miss = Number(s.keyspace_misses || 0)
  const total = hits + miss
  return total ? ((hits / total) * 100).toFixed(1) + '%' : '-'
})

async function loadSlowlog() {
  try { slowlog.value = (await A.GetSlowLog(cid(), props.db, 25)) || [] } catch (e) { alert(String(e)) }
}
async function loadClients() {
  try { clients.value = (await A.GetClientList(cid(), props.db)) || [] } catch (e) { alert(String(e)) }
}

async function subscribe() {
  if (!pubChannel.value) return
  try {
    pubSession.value = await A.PubSubSubscribe(cid(), pubChannel.value)
  } catch (e) { alert(String(e)) }
}
function unsubscribe() {
  if (pubSession.value) A.PubSubUnsubscribe(pubSession.value)
  pubSession.value = ''
}
function onPubMsg(connID, channel, tuple) {
  if (connID !== cid()) return
  pubMessages.value.unshift({ ts: new Date().toLocaleTimeString(), channel: tuple[1], payload: tuple[2] })
  if (pubMessages.value.length > 100) pubMessages.value.pop()
}

function sparkline(series, color, fmt) {
  // simple inline SVG polyline
  const w = 240, h = 48
  const max = Math.max(...series, 1)
  const pts = series.map((v, i) =>
    `${(i / Math.max(series.length - 1, 1)) * w},${h - (v / max) * (h - 4) - 2}`).join(' ')
  return { points: pts, w, h, color, maxLabel: fmt ? fmt(max) : String(max) }
}

onMounted(() => {
  poll()
  pollTimer = setInterval(poll, 1000)
  EventsOn(EVENTS.PUBSUB_MESSAGE, onPubMsg)
})
onUnmounted(() => {
  clearInterval(pollTimer)
  EventsOff(EVENTS.PUBSUB_MESSAGE)
  unsubscribe()
})
</script>

<template>
  <div class="sv">
    <div class="tabs">
      <button v-for="t in ['info','slowlog','clients','pubsub']" :key="t"
              :class="{ active: tab === t }"
              @click="tab = t; t === 'slowlog' && loadSlowlog(); t === 'clients' && loadClients()">
        {{ t }}
      </button>
      <span class="flex1"></span>
      <button @click="poll()">刷新</button>
    </div>

    <template v-if="tab === 'info'">
      <p v-if="infoErr" class="err">{{ infoErr }}</p>
      <div class="cards" v-if="info">
        <div class="card" v-for="c in cards" :key="c.label">
          <div class="lbl">{{ c.label }}</div>
          <div class="val">{{ c.v }}</div>
        </div>
      </div>
      <div class="charts" v-if="history.length > 1">
        <div class="chart-box">
          <div class="chart-title">内存 <span class="max">{{ sparkline(memSeries, '#4a90d9', fmtMem).maxLabel }}</span></div>
          <svg :viewBox="`0 0 ${sparkline(memSeries, '#4a90d9').w} ${sparkline(memSeries, '#4a90d9').h}`" class="spark">
            <polyline :points="sparkline(memSeries, '#4a90d9').points" fill="none" stroke="#4a90d9" stroke-width="1.5" />
          </svg>
        </div>
        <div class="chart-box">
          <div class="chart-title">ops/sec <span class="max">{{ sparkline(opsSeries, '#4aa87c').maxLabel }}</span></div>
          <svg :viewBox="`0 0 ${sparkline(opsSeries, '#4aa87c').w} ${sparkline(opsSeries, '#4aa87c').h}`" class="spark">
            <polyline :points="sparkline(opsSeries, '#4aa87c').points" fill="none" stroke="#4aa87c" stroke-width="1.5" />
          </svg>
        </div>
      </div>
    </template>

    <template v-else-if="tab === 'slowlog'">
      <table>
        <thead><tr><th>id</th><th>时间</th><th>耗时(ms)</th><th>命令</th><th>client</th></tr></thead>
        <tbody>
          <tr v-for="e in slowlog" :key="e.id">
            <td>{{ e.id }}</td>
            <td>{{ new Date(e.timestamp * 1000).toLocaleString() }}</td>
            <td>{{ (e.duration_us / 1000).toFixed(2) }}</td>
            <td class="mono">{{ e.command }}</td>
            <td>{{ e.client }}</td>
          </tr>
          <tr v-if="!slowlog.length"><td colspan="5" class="empty">无记录</td></tr>
        </tbody>
      </table>
    </template>

    <template v-else-if="tab === 'clients'">
      <table>
        <thead><tr><th>id</th><th>addr</th><th>name</th><th>db</th><th>age</th><th>idle</th><th>cmd</th></tr></thead>
        <tbody>
          <tr v-for="c in clients" :key="c.id">
            <td>{{ c.id }}</td><td class="mono">{{ c.addr }}</td><td>{{ c.name }}</td>
            <td>{{ c.db }}</td><td>{{ c.age }}</td><td>{{ c.idle }}</td><td class="mono">{{ c.cmd }}</td>
          </tr>
          <tr v-if="!clients.length"><td colspan="7" class="empty">无</td></tr>
        </tbody>
      </table>
    </template>

    <template v-else>
      <div class="pub-row">
        <input v-model="pubChannel" placeholder="频道名，如 news" @keyup.enter="subscribe" :disabled="!!pubSession" />
        <button v-if="!pubSession" @click="subscribe">订阅</button>
        <button v-else @click="unsubscribe">取消订阅</button>
      </div>
      <div class="pub-feed">
        <div v-for="(m, i) in pubMessages" :key="i" class="pub-msg">
          <span class="ts">{{ m.ts }}</span>
          <b>{{ m.channel }}</b>
          <span class="mono">{{ m.payload }}</span>
        </div>
        <p v-if="!pubMessages.length" class="empty">订阅后消息将显示在这里</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.sv { display: flex; flex-direction: column; gap: 12px; height: 100%; overflow: auto; }
.tabs { display: flex; gap: 6px; }
.tabs button.active { background: #3567b3; border-color: #3567b3; color: #fff; }
.flex1 { flex: 1; }
.cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; }
.card { border: 1px solid var(--border); border-radius: 10px; padding: 10px 14px; }
.lbl { color: var(--muted); font-size: 11px; text-transform: uppercase; }
.val { font-size: 18px; font-weight: 600; }
.charts { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.chart-box { border: 1px solid var(--border); border-radius: 10px; padding: 10px 14px; }
.chart-title { font-size: 12px; color: var(--muted); }
.chart-title .max { float: right; color: var(--muted); }
.spark { width: 100%; height: 48px; }
table { width: 100%; border-collapse: collapse; font-size: 12.5px; }
th, td { text-align: left; padding: 5px 8px; border-bottom: 1px solid var(--border); }
th { color: var(--muted); font-weight: 500; }
.mono { font-family: 'SF Mono', Menlo, monospace; word-break: break-all; }
.empty { color: var(--muted); text-align: center; padding: 20px; }
.pub-row { display: flex; gap: 8px; }
.pub-row input { flex: 1; }
.pub-feed { flex: 1; overflow: auto; display: flex; flex-direction: column; gap: 4px; }
.pub-msg { padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px; display: flex; gap: 10px; }
.ts { color: var(--muted); font-size: 11px; }
button { padding: 5px 12px; border: 1px solid #ccc; border-radius: 6px; background: transparent; cursor: pointer; }
button:hover { filter: brightness(.95); }
input, select { padding: 6px 8px; border: 1px solid #ccc; border-radius: 6px; }
.err { color: #d82c20; font-size: 12px; }
</style>
