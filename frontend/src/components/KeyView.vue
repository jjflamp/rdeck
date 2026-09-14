<script setup>
// Value editor — the Go-rewrite counterpart of ValueTabs / ValueTable /
// MultilineEditor (P2 MVP scope: browse + row edit + string editor +
// formatters + transparent compression).
import { ref, computed, onMounted, onUnmounted, inject } from 'vue'

const props = defineProps({
  connID: { type: String, required: true },
  db: { type: Number, required: true },
  node: { type: Object, required: true },   // tree node of the key
  connCfg: { type: Object, required: false, default: null }, // full config — source of truth for the id
  retryConn: { type: Function, required: false, default: null }, // self-heal hook
})

// The id from the full config is the source of truth; props.connID has
// historically gone stale/empty in some paths.
const cid = () => props.connCfg?.id || props.connID

// Ensure the manager still holds a live connection before every batch of
// backend calls (cheap map lookup; reopens when evicted).
async function ensureConn() {
  if (props.connCfg) await A.EnsureConnection(props.connCfg)
}
const emit = defineEmits(['close', 'changed'])

const A = window.go.app.App
const notify = inject('notify', () => {})

const meta = ref(null)
const rows = ref([])
const cursor = ref('') // current page cursor ('' = start)
const loading = ref(false)
const error = ref('')
const match = ref('')
const limit = 100

// editor modal state
const editing = ref(null)      // row being edited (or {isNew:true})
const editorText = ref('')
const formatterName = ref('plain')
const decodeError = ref('')
const compressedAlg = ref('')  // detected compression of the raw value
const saveError = ref('')
const saving = ref(false)

const formatters = ref([])
const algs = ref([])

// In-app confirm/prompt — window.confirm/prompt are unreliable inside the
// WKWebView (silently return false/null), which made delete buttons no-op.
const askDialog = ref(null) // {type:'confirm'|'prompt', message, value, resolve}
function askConfirm(message) {
  return new Promise(resolve => { askDialog.value = { type: 'confirm', message, resolve } })
}
function askPrompt(message, defaultValue = '') {
  return new Promise(resolve => { askDialog.value = { type: 'prompt', message, value: defaultValue, resolve } })
}
function answerAsk(v) {
  const d = askDialog.value
  askDialog.value = null
  d.resolve(v)
}

const isString = computed(() => ['string', 'ReJSON-RL', 'ReJSON'].includes(props.node.keyType))

const columns = computed(() => {
  switch (props.node.keyType) {
    case 'hash': return [{ k: 'key', t: 'field' }, { k: 'value', t: 'value' }]
    case 'list': return [{ k: 'index', t: '#' }, { k: 'value', t: 'value' }]
    case 'set': return [{ k: 'key', t: 'member' }]
    case 'zset': return [{ k: 'key', t: 'member' }, { k: 'score', t: 'score' }]
    case 'stream': return [{ k: 'key', t: 'id' }, { k: 'value', t: 'fields' }]
    default: return []
  }
})

async function loadMeta() {
  meta.value = await A.OpenKey(cid(), props.db, props.node.fullPath, props.node.keyType)
}

async function loadFirst() {
  rows.value = []
  cursor.value = ''
  await loadMore()
  if (isString.value) await refreshStringEditor()
}

// Full reload: meta + all rows from the first page (刷新按钮).
async function reloadKey() {
  loading.value = true
  try {
    await loadMeta()
    await loadFirst()
  } catch (e) {
    error.value = String(e && e.message ? e.message : e)
  }
  loading.value = false
}

async function loadMore() {
  loading.value = true
  error.value = ''
  try {
    await ensureConn()
    const page = await A.LoadKeyRows(cid(), props.db, props.node.fullPath,
                                     props.node.keyType, cursor.value, match.value || '*', limit)
    rows.value.push(...(page.rows || []))
    cursor.value = page.nextCursor || ''
  } catch (e) {
    error.value = String(e)
  }
  loading.value = false
}

async function loadFullValue() {
  loading.value = true
  error.value = ''
  try {
    const fullB64 = await A.GetFullValue(cid(), props.db, props.node.fullPath)
    if (rows.value[0]) rows.value[0].value = fullB64
    if (meta.value) meta.value.truncated = false
    await refreshStringEditor()
  } catch (e) {
    error.value = String(e)
  }
  loading.value = false
}

// --- string inline editor ----------------------------------------------------
// String keys edit in-place; compression is handled transparently (decompress
// for display, recompress on save — same rule as the row editor modal).
const stringText = ref('')
const stringAlg = ref('')      // detected compression of the stored value
const stringSaving = ref(false)

async function refreshStringEditor() {
  stringAlg.value = ''
  const rawB64 = rows.value[0]?.value || ''
  let cur = rawB64
  try {
    const alg = await A.DetectCompression(rawB64)
    if (alg) {
      cur = await A.DecompressValue(alg, rawB64)
      stringAlg.value = alg
    }
  } catch { /* not compressed */ }
  stringText.value = b64ToText(cur)
}

async function saveString() {
  if (stringSaving.value) return
  stringSaving.value = true
  error.value = ''
  try {
    let valueB64 = textToB64(stringText.value)
    if (stringAlg.value) {
      valueB64 = await A.CompressValue(stringAlg.value, valueB64)
    }
    await A.SaveStringValue(cid(), props.db, props.node.fullPath, valueB64)
    await loadMeta()
    emit('changed')
    notify('value 已保存')
  } catch (e) {
    const msg = String(e && e.message ? e.message : e)
    error.value = msg
    notify('保存失败: ' + msg, 'err')
  }
  stringSaving.value = false
}

async function setTTL() {
  const input = await askPrompt('TTL 秒数（0 = 移除过期）', String(meta.value?.ttl ?? -1))
  if (input === null) return
  try {
    await A.SetKeyTTL(cid(), props.db, props.node.fullPath, parseInt(input, 10) || 0)
    await loadMeta()
    emit('changed')
    notify?.('TTL 已更新')
  } catch (e) { error.value = String(e); notify?.(String(e), 'err') }
}

async function renameKey() {
  const to = await askPrompt('新 key 名称', props.node.fullPath)
  if (!to || to === props.node.fullPath) return
  try {
    await A.RenameKey(cid(), props.db, props.node.fullPath, to)
    emit('changed')
    emit('close')
    notify?.('已重命名')
  } catch (e) { error.value = String(e); notify?.(String(e), 'err') }
}

async function deleteKey() {
  if (!(await askConfirm(`删除 key ${props.node.fullPath}?`))) return
  try {
    await A.DeleteKey(cid(), props.db, props.node.fullPath)
    emit('changed')
    emit('close')
    notify?.('key 已删除')
  } catch (e) { error.value = String(e); notify?.(String(e), 'err') }
}

// --- row editor ---------------------------------------------------------------

function b64ToText(b64) {
  try { return decodeURIComponent(escape(atob(b64))) } catch { return atob(b64) }
}
function textToB64(t) { return btoa(unescape(encodeURIComponent(t))) }

function autoFormatter(rawB64) {
  const r = A.DecodeValue('json', rawB64)
  if (!r.error) return 'json'
  return 'plain'
}

async function openEditor(row) {
  editing.value = row
  saveError.value = ''
  decodeError.value = ''
  compressedAlg.value = ''
  let rawB64 = row ? row.value : ''

  // transparent decompression
  try {
    const alg = await A.DetectCompression(rawB64)
    if (alg) {
      rawB64 = await A.DecompressValue(alg, rawB64)
      compressedAlg.value = alg
    }
  } catch { /* detection failure = not compressed */ }
  editing.value = { ...row, _raw: rawB64 }

  formatterName.value = isString.value || props.node.keyType === 'stream' ? autoFormatter(rawB64) : 'plain'

  if (formatterName.value !== 'plain') {
    const r = await A.DecodeValue(formatterName.value, rawB64)
    if (r.error) { decodeError.value = r.error; editorText.value = b64ToText(rawB64) }
    else editorText.value = r.output
  } else {
    editorText.value = b64ToText(rawB64)
  }
}

async function switchFormatter(name) {
  // Re-decode from the stored raw value; unsaved text is discarded.
  decodeError.value = ''
  if (!editing.value?._raw) return
  if (name === 'plain') {
    editorText.value = b64ToText(editing.value._raw)
    return
  }
  const r = await A.DecodeValue(name, editing.value._raw)
  if (r.error) {
    decodeError.value = r.error
    editorText.value = b64ToText(editing.value._raw)
  } else {
    editorText.value = r.output
  }
}

async function saveEditor() {
  saving.value = true
  saveError.value = ''
  try {
    let valueB64 = textToB64(editorText.value)
    if (formatterName.value !== 'plain') {
      const enc = await A.EncodeValue(formatterName.value, editorText.value)
      if (enc === null) throw new Error('编码失败：内容不是合法的 ' + formatterName.value)
      valueB64 = enc
    }
    if (compressedAlg.value) {
      valueB64 = await A.CompressValue(compressedAlg.value, valueB64)
    }

    if (isString.value) {
      await A.SaveStringValue(cid(), props.db, props.node.fullPath, valueB64)
    } else {
      // set/zset members are immutable: an edited text is a member rename
      // (SREM+SADD / ZREM+ZADD via the newKey field).
      let newKey = ''
      if (['set', 'zset'].includes(props.node.keyType) && editorText.value !== editing.value.key) {
        newKey = editorText.value
      }
      await A.EditRow(cid(), props.db, props.node.fullPath, {
        op: 'update', type: props.node.keyType,
        key: editing.value.key, newKey, index: editing.value.index,
        value: valueB64, score: editing.value.score,
      })
    }
    editing.value = null
    await Promise.all([loadMeta(), loadFirst()])
    emit('changed')
    notify('已保存')
  } catch (e) {
    const msg = String(e && e.message ? e.message : e)
    saveError.value = msg
    notify('保存失败: ' + msg, 'err')
  }
  saving.value = false
}

async function removeRow(row) {
  if (!(await askConfirm('删除该行?'))) return
  try {
    await A.EditRow(cid(), props.db, props.node.fullPath, {
      op: 'remove', type: props.node.keyType, key: row.key, index: row.index, value: row.value,
    })
    await Promise.all([loadMeta(), loadFirst()])
    emit('changed')
    notify('行已删除')
  } catch (e) { notify(String(e), 'err') }
}

// new-row dialog state
const showNewRow = ref(false)
const newKeyField = ref('')
const newScore = ref(0)
const newValue = ref('')
function openNewRow() { showNewRow.value = true; newKeyField.value = ''; newScore.value = 0; newValue.value = '' }
async function saveNewRow() {
  try {
    await A.EditRow(cid(), props.db, props.node.fullPath, {
      op: 'add', type: props.node.keyType,
      key: newKeyField.value, index: 0,
      value: textToB64(newValue.value), score: Number(newScore.value),
    })
    showNewRow.value = false
    await Promise.all([loadMeta(), loadFirst()])
    emit('changed')
    notify('行已添加')
  } catch (e) { notify(String(e), 'err') }
}

function cellText(row, k) {
  if (k === 'value') return b64ToText(row.value)
  if (k === 'score') return row.score
  return row[k]
}

// Esc inside KeyView modals must not bubble to the app-level handler
// (which would navigate back to the tree).
function onKey(e) {
  if (e.key !== 'Escape') return
  if (askDialog.value) {
    const d = askDialog.value
    askDialog.value = null
    d.resolve(d.type === 'prompt' ? null : false)
    e.stopPropagation()
  } else if (editing.value || showNewRow.value) {
    editing.value = null
    showNewRow.value = false
    e.stopPropagation()
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKey, true)
  try {
    formatters.value = await A.ListFormatters()
    algs.value = await A.ListCompressionAlgs()
    await ensureConn()
    await loadMeta()
    await loadFirst()
  } catch (e) {
    const msg = String(e && e.message ? e.message : e)
    // Self-heal: the manager lost the connection — reconnect once and retry.
    if (msg.includes('is not open') && props.retryConn) {
      try {
        await props.retryConn()
        await loadMeta()
        await loadFirst()
        error.value = ''
        loading.value = false
        return
      } catch (e2) {
        error.value = String(e2 && e2.message ? e2.message : e2)
        loading.value = false
        return
      }
    }
    error.value = msg + `  [conn=${cid() || '(空)'} db=${props.db}]`
    loading.value = false
  }
})
onUnmounted(() => window.removeEventListener('keydown', onKey, true))
</script>

<template>
  <div class="kv">
    <div class="kv-head">
      <button @click="emit('close')">← 返回</button>
      <b class="kv-name">{{ node.fullPath }}</b>
      <span class="type-badge" v-if="node.keyType">{{ node.keyType }}</span>
      <template v-if="meta">
        <span class="meta">TTL: {{ meta.ttl < 0 ? '∞' : meta.ttl + 's' }}</span>
        <span class="meta">rows: {{ meta.rows }}</span>
        <span class="meta" v-if="meta.size">size: {{ meta.size }}B</span>
        <span class="meta warn" v-if="meta.truncated">已截断（值过大）</span>
      </template>
      <span class="flex1"></span>
      <button class="sm" @click="reloadKey" :disabled="loading" title="重新加载数据">🔄 刷新</button>
      <button @click="setTTL">TTL</button>
      <button @click="renameKey">重命名</button>
      <button class="danger" @click="deleteKey">删除 key</button>
      <button v-if="!isString" class="primary" @click="openNewRow">＋ 添加行</button>
    </div>

    <p v-if="error" class="err">{{ error }}</p>

    <!-- string editor -->
    <template v-if="isString">
      <div class="str-toolbar">
        <button v-if="meta?.truncated" class="sm" @click="loadFullValue" :disabled="loading">
          ⬇ 加载完整值（当前已截断）
        </button>
        <span class="hint" v-if="stringAlg">已自动解压: {{ stringAlg }}（保存时自动重新压缩）</span>
        <span class="flex1"></span>
        <button class="primary" @click="saveString" :disabled="stringSaving">
          {{ stringSaving ? '保存中…' : '保存' }}
        </button>
      </div>
      <textarea v-model="stringText" class="editor" spellcheck="false" />
    </template>

    <!-- multi-row table -->
    <template v-else>
      <div class="toolbar" v-if="['hash','set','zset'].includes(node.keyType)">
        <input v-model="match" :placeholder="node.keyType === 'hash' ? '过滤 field pattern' : '过滤 member pattern'"
               @keyup.enter="loadFirst" />
        <button @click="loadFirst">过滤</button>
      </div>
      <table>
        <thead>
          <tr><th v-for="c in columns" :key="c.k">{{ c.t }}</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.index + (row.key || '')">
            <td v-for="c in columns" :key="c.k" class="cell"
                @click="c.k === 'value' || c.k === 'key' ? openEditor(row) : null">
              {{ cellText(row, c.k) }}
            </td>
            <td><button class="danger sm" @click="removeRow(row)">删</button></td>
          </tr>
        </tbody>
      </table>
      <div class="load-more" v-if="cursor">
        <button @click="loadMore" :disabled="loading">{{ loading ? '加载中…' : '加载更多' }}</button>
      </div>
    </template>

    <!-- edit modal -->
    <div class="modal-mask" v-if="editing">
      <div class="modal">
        <h4>{{ editing.isNew ? '编辑值' : `编辑 ${editing.key || '#' + editing.index}` }}</h4>
        <p v-if="compressedAlg" class="ok">已自动解压: {{ compressedAlg }}（保存时自动重新压缩）</p>
        <p v-if="decodeError" class="err">{{ decodeError }}（按原文编辑）</p>
        <div class="fmt-row" v-if="!editing.isNew">
          <label>Formatter</label>
          <select v-model="formatterName" @change="switchFormatter(formatterName)">
            <option value="plain">plain text</option>
            <option v-for="f in formatters" :key="f.name" :value="f.name">{{ f.description }}</option>
          </select>
        </div>
        <textarea v-model="editorText" class="editor" />
        <p v-if="saveError" class="err">{{ saveError }}</p>
        <div class="btn-row">
          <button class="primary" @click="saveEditor" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</button>
          <button @click="editing = null">取消</button>
        </div>
      </div>
    </div>

    <!-- new row modal -->
    <div class="modal-mask" v-if="showNewRow">      <div class="modal">
        <h4>添加行 · {{ node.keyType }}</h4>
        <div class="grid" v-if="['hash','set','zset','stream'].includes(node.keyType)">
          <label>{{ node.keyType === 'stream' ? 'ID（留空自动）' : node.keyType === 'hash' ? 'field' : 'member' }}
            <input v-model="newKeyField" /></label>
          <label v-if="node.keyType === 'zset'">score<input v-model.number="newScore" type="number" /></label>
        </div>
        <label>value
          <textarea v-model="newValue" class="editor"
                    :placeholder="node.keyType === 'stream' ? 'JSON 对象，如 {&quot;field&quot;:&quot;value&quot;}' : ''" />
        </label>
        <div class="btn-row">
          <button class="primary" @click="saveNewRow">添加</button>
          <button @click="showNewRow = false">取消</button>
        </div>
      </div>
    </div>

    <!-- in-app confirm/prompt -->
    <div class="modal-mask" v-if="askDialog" @click.self="answerAsk(askDialog.type === 'prompt' ? null : false)">
      <div class="modal ask">
        <p class="ask-msg">{{ askDialog.message }}</p>
        <input v-if="askDialog.type === 'prompt'" v-model="askDialog.value"
               @keyup.enter="answerAsk(askDialog.value || '')" autofocus />
        <div class="btn-row">
          <button class="primary" @click="answerAsk(askDialog.type === 'prompt' ? (askDialog.value || '') : true)">确定</button>
          <button @click="answerAsk(askDialog.type === 'prompt' ? null : false)">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.kv { display: flex; flex-direction: column; gap: 10px; height: 100%; }
.kv-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.kv-name { font-family: 'SF Mono', Menlo, monospace; word-break: break-all; }
.flex1 { flex: 1; }
.meta { color: var(--muted); font-size: 12px; }
.warn { color: #c98a3d; }
.type-badge { background: #d4a017; color: #fff; font-size: 10px; padding: 2px 8px; border-radius: 8px; text-transform: uppercase; }
.toolbar { display: flex; gap: 8px; }
.toolbar input { flex: 1; }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th, td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--border); }
th { color: var(--muted); font-weight: 500; font-size: 12px; }
.cell { cursor: pointer; max-width: 480px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: 'SF Mono', Menlo, monospace; }
.cell:hover { background: rgba(128,128,128,.08); }
.editor { width: 100%; min-height: 300px; font-family: 'SF Mono', Menlo, monospace; font-size: 13px;
          padding: 10px; border: 1px solid var(--border); border-radius: 8px; resize: vertical; }
.load-more { text-align: center; }
button { padding: 6px 12px; border: 1px solid var(--border); border-radius: 6px; background: transparent; cursor: pointer; }
button.primary { background: #d82c20; border-color: #d82c20; color: #fff; }
button.danger { color: #d82c20; }
button.sm { padding: 2px 8px; font-size: 12px; }
.err { color: #d82c20; font-size: 12px; }
.ok { color: #2a7; font-size: 12px; }
.str-toolbar { display: flex; align-items: center; gap: 10px; }
.str-toolbar .hint { color: #2a7; font-size: 12px; }
.flex1 { flex: 1; }
.modal-mask { position: fixed; inset: 0; background: rgba(0,0,0,.4); display: flex; align-items: center; justify-content: center; z-index: 10; }
.modal { background: var(--panel); border-radius: 12px; padding: 20px; width: min(860px, 90vw); max-height: 85vh; overflow: auto; }
.modal .editor { min-height: 240px; }
.modal.ask { width: min(420px, 80vw); }
.ask-msg { word-break: break-all; font-size: 13px; margin: 4px 0 10px; }
.modal input { width: 100%; padding: 7px 9px; border: 1px solid var(--border); border-radius: 6px;
               background: var(--panel); color: var(--fg); font-family: 'SF Mono', Menlo, monospace; }
.fmt-row { display: flex; align-items: center; gap: 8px; margin: 8px 0; }
.fmt-row select { padding: 4px 8px; border-radius: 6px; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin: 10px 0; }
label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
input, select { padding: 6px 8px; border: 1px solid var(--border); border-radius: 6px; }
.btn-row { display: flex; gap: 10px; margin-top: 12px; }
</style>
