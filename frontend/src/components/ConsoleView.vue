<script setup>
// Redis console — dedicated raw session, history, autocomplete from
// commands.json, MONITOR toggle.
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { EVENTS } from '../api/events'
import commands from '../assets/commands.json'

const { EventsOn, EventsOff } = window.runtime

const props = defineProps({
  connID: { type: String, required: true },
  db: { type: Number, required: true },
  connCfg: { type: Object, required: false, default: null },
  retryConn: { type: Function, required: false, default: null },
})
const emit = defineEmits(['close'])

const A = window.go.app.App
const cid = () => props.connCfg?.id || props.connID

const output = ref('')
const input = ref('')
const sessionID = ref('')
const monitoring = ref(false)
const history = ref([])
const historyIdx = ref(-1)
const outEl = ref(null)
const suggest = ref([])
const suggestIdx = ref(-1)

const cmdIndex = commands.map(c => ({ cmd: c.cmd, summary: (c.summary || '').trim() }))

function append(line) {
  output.value += (output.value ? '\n' : '') + line
  nextTick(() => outEl.value && (outEl.value.scrollTop = outEl.value.scrollHeight))
}

async function exec(line) {
  append(`${props.db}> ${line}`)
  // Lazy self-heal: session lost (e.g. failed open) — reopen before executing.
  if (!sessionID.value && props.retryConn) {
    try { await openSession() } catch (e) { append('-- ' + e + ' --'); return }
  }
  try {
    const out = await A.ConsoleExecute(sessionID.value, line)
    if (out) append(out)
  } catch (e) {
    append(String(e))
  }
}

async function submit() {
  const line = input.value.trim()
  if (!line) return
  suggest.value = []
  input.value = ''
  history.value.push(line)
  historyIdx.value = history.value.length
  if (line.toLowerCase() === 'monitor') {
    append('提示：请用 MONITOR 开关按钮进入监控模式')
  }
  await exec(line)
}

async function toggleMonitor() {
  try {
    if (!monitoring.value) {
      await A.ConsoleStartMonitor(sessionID.value)
      monitoring.value = true
      append('-- MONITOR 已开启 --')
    } else {
      monitoring.value = false
      append('-- 请断开后重开会话以停止 MONITOR（专用连接限制） --')
      emit('close') // force session recycle
    }
  } catch (e) {
    append(String(e))
  }
}

function onMonitorData(sid, lines) {
  if (sid !== sessionID.value) return
  append(lines.join('\n'))
}

function onKey(e) {
  if (e.key === 'ArrowUp') {
    if (historyIdx.value > 0) { historyIdx.value--; input.value = history.value[historyIdx.value] }
    e.preventDefault()
  } else if (e.key === 'ArrowDown') {
    if (historyIdx.value < history.value.length - 1) { historyIdx.value++; input.value = history.value[historyIdx.value] }
    else { historyIdx.value = history.value.length; input.value = '' }
    e.preventDefault()
  } else if (e.key === 'Tab' && suggest.value.length) {
    input.value = suggest.value[suggestIdx.value < 0 ? 0 : suggestIdx.value].cmd
    e.preventDefault()
  } else if (e.key === 'Escape') {
    suggest.value = []
  }
}

function updateSuggest() {
  const word = input.value.trim().split(/\s+/)[0] || ''
  if (!word || input.value.includes(' ')) { suggest.value = []; return }
  const w = word.toLowerCase()
  suggest.value = cmdIndex.filter(c => c.cmd.toLowerCase().startsWith(w)).slice(0, 8)
  suggestIdx.value = -1
}

function pickSuggestion(s) {
  input.value = s.cmd + ' '
  suggest.value = []
  outEl.value?.focus?.()
}

async function openSession() {
  sessionID.value = await A.OpenConsole(cid(), props.db)
  append(`-- 已连接（db${props.db}）。支持 Tab 补全、↑↓ 历史 --`)
}

onMounted(async () => {
  try {
    await openSession()
  } catch (e) {
    const msg = String(e && e.message ? e.message : e)
    if (msg.includes('is not open') && props.retryConn) {
      try {
        await props.retryConn()
        await openSession()
      } catch (e2) {
        append('-- 打开会话失败: ' + (e2.message || e2) + ' --')
      }
    } else {
      append('-- 打开会话失败: ' + msg + ' --')
    }
  }
  EventsOn(EVENTS.CONSOLE_MONITOR, onMonitorData)
})
onUnmounted(() => {
  EventsOff(EVENTS.CONSOLE_MONITOR)
  if (sessionID.value) A.CloseConsole(sessionID.value)
})
</script>

<template>
  <div class="console">
    <div class="cons-head">
      <b>Console</b>
      <button :class="{ active: monitoring }" @click="toggleMonitor">MONITOR</button>
      <button @click="emit('close')">关闭</button>
    </div>
    <div class="out" ref="outEl">{{ output }}</div>
    <div class="suggest" v-if="suggest.length">
      <div v-for="(s, i) in suggest" :key="s.cmd" class="sg"
           :class="{ sel: i === suggestIdx }" @mousedown.prevent="pickSuggestion(s)">
        <b>{{ s.cmd }}</b><span class="sum">{{ s.summary }}</span>
      </div>
    </div>
    <div class="in-row">
      <span class="prompt">db{{ db }}&gt;</span>
      <input v-model="input" @keydown="onKey" @input="updateSuggest" @keyup.enter="submit"
             placeholder="输入 Redis 命令…" autofocus spellcheck="false" />
    </div>
  </div>
</template>

<style scoped>
.console { display: flex; flex-direction: column; gap: 8px; height: 100%; }
.cons-head { display: flex; align-items: center; gap: 10px; }
.cons-head button.active { background: #d82c20; color: #fff; border-color: #d82c20; }
.out { flex: 1; overflow: auto; background: #14161a; color: #c9e3c9; font-family: 'SF Mono', Menlo, monospace;
       font-size: 12.5px; padding: 12px; border-radius: 8px; white-space: pre-wrap; word-break: break-all; }
.in-row { display: flex; align-items: center; gap: 8px; }
.prompt { color: #d82c20; font-family: 'SF Mono', Menlo, monospace; font-weight: 600; }
.in-row input { flex: 1; font-family: 'SF Mono', Menlo, monospace; padding: 8px 10px;
                border: 1px solid #ccc; border-radius: 6px; background: #14161a; color: #e6e8eb; }
.suggest { border: 1px solid rgba(128,128,128,.3); border-radius: 8px; overflow: hidden; max-height: 220px; overflow-y: auto; }
.sg { display: flex; gap: 12px; padding: 5px 10px; cursor: pointer; align-items: baseline; }
.sg:hover, .sg.sel { background: rgba(128,128,128,.12); }
.sg .sum { color: #888; font-size: 12px; }
button { padding: 5px 12px; border: 1px solid #ccc; border-radius: 6px; background: transparent; cursor: pointer; }
button:hover { filter: brightness(.95); }
</style>
