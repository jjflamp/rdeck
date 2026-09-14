<script setup>
import { ref, onMounted, provide } from 'vue'
import { useI18n } from 'vue-i18n'
import VirtualTree from './components/VirtualTree.vue'
import KeyView from './components/KeyView.vue'
import ConsoleView from './components/ConsoleView.vue'
import ServerView from './components/ServerView.vue'
import BulkDialog from './components/BulkDialog.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import { applyLocale } from './i18n'

const { t } = useI18n()

const { ListConnections, SaveConnection, DeleteConnection, TestConnection, ImportFromRDM,
        OpenConnection, EnsureConnection, Disconnect, RefreshKeys, GetSettings, SetSettings,
        GetFilterHistory, SaveFilterHistory } = window.go.app.App

const connections = ref([])
const editing = ref(null)
const testing = ref(false)
const testResult = ref(null)
const importPath = ref('')
const message = ref('')

// browse / view state
const opened = ref(null)
const opening = ref(false)
const activeDb = ref(0)
const filter = ref('')
const tree = ref(null)
const loadingTree = ref(false)
const selectedKey = ref(null)
const viewTab = ref('browse')
const showBulk = ref(false)
const showSettings = ref(false)

// --- global toast notifications (P7 UX) ---------------------------------------
const toasts = ref([])
let toastSeq = 0
function notify(msg, type = 'ok') {
  const id = ++toastSeq
  toasts.value.push({ id, msg, type })
  setTimeout(() => { toasts.value = toasts.value.filter(t => t.id !== id) }, 3000)
}
provide('notify', notify)

// --- connection delete confirm (in-app, was one-click!) ------------------------
const pendingDel = ref(null)
function askDeleteConn(c) { pendingDel.value = c }
async function doDeleteConn() {
  const c = pendingDel.value
  pendingDel.value = null
  if (!c) return
  if (opened.value?.id === c.id) { opened.value = null; tree.value = null; selectedKey.value = null }
  const m = { ...connMeta.value }; delete m[c.id]; connMeta.value = m
  const e2 = { ...expandedConns.value }; delete e2[c.id]; expandedConns.value = e2
  await DeleteConnection(c.id)
  await reload()
  notify(`已删除连接 ${c.name || c.host}`)
}

// Esc closes dialogs / returns from a key view
function onGlobalKey(e) {
  if (e.key !== 'Escape') return
  if (pendingDel.value) { pendingDel.value = null; return }
  if (showSettings.value) { showSettings.value = false; return }
  if (showBulk.value) { showBulk.value = false; return }
  if (selectedKey.value && viewTab.value === 'browse') selectedKey.value = null
}

// sidebar connection → db tree
const expandedConns = ref({})   // connID -> bool
const connMeta = ref({})        // connID -> ConnSummary (db list cache)

// theme / locale (P5)
const darkMode = ref(false)
const localeSetting = ref('system')

async function loadPrefs() {
  try {
    const s = await GetSettings()
    window.__cachedSettings = s
    darkMode.value = !!s.dark_mode
    localeSetting.value = s.locale || 'system'
    document.body.classList.toggle('dark', darkMode.value)
    applyLocale(localeSetting.value)
  } catch { /* defaults */ }
}

function savePrefs(patch) {
  if (!window.__cachedSettings) return
  window.__cachedSettings = { ...window.__cachedSettings, ...patch }
  SetSettings(window.__cachedSettings)
}

function toggleDark() {
  darkMode.value = !darkMode.value
  document.body.classList.toggle('dark', darkMode.value)
  savePrefs({ dark_mode: darkMode.value })
}

function cycleLocale() {
  localeSetting.value = localeSetting.value === 'zh_CN' ? 'en' : 'zh_CN'
  applyLocale(localeSetting.value)
  savePrefs({ locale: localeSetting.value })
}

async function reload() {
  connections.value = await ListConnections()
}

function blank() {
  return {
    id: '', name: '', host: '127.0.0.1', port: 6379,
    auth: '', username: '',
    useSSL: false, sslCACertPath: '', sslPrivateKeyPath: '', sslLocalCertPath: '', sslIgnoreAllErrors: false,
    useSSHTunnel: false, sshHost: '', sshPort: 22, sshUser: '', sshPassword: '',
    sshPrivateKeyPath: '', sshAgent: false, askForSSHPassword: false,
    keysPattern: '*', namespaceSeparator: ':',
  }
}

function edit(cfg) {
  editing.value = cfg ? JSON.parse(JSON.stringify(cfg)) : blank()
  testResult.value = null
}

async function save() {
  try {
    await SaveConnection(editing.value)
    // edited config invalidates the cached db list / open connection
    if (connMeta.value[editing.value.id]) {
      const m = { ...connMeta.value }
      delete m[editing.value.id]
      connMeta.value = m
      Disconnect(editing.value.id)
      if (opened.value?.id === editing.value.id) { opened.value = null; tree.value = null }
    }
    editing.value = null
    message.value = t('app.saved')
    await reload()
  } catch (e) {
    message.value = String(e)
  }
}

async function del(id) {
  // no-op: replaced by askDeleteConn/doDeleteConn (confirm before delete)
}

async function test(cfg) {
  const target = cfg || editing.value
  if (!target) return
  testing.value = true
  testResult.value = null
  let result
  try {
    result = await TestConnection(target)
  } catch (e) {
    // backend sentinel surfaces as string via IPC: offer password prompt + retry
    if (String(e).includes('ssh password required')) {
      const pw = prompt(`SSH 密码（${target.sshHost || target.ssh_host || ''}）:`)
      if (pw !== null) {
        try {
          result = await TestConnection({ ...target, sshPassword: pw })
        } catch (e2) {
          result = { ok: false, error: String(e2) }
        }
      } else {
        result = { ok: false, error: '已取消' }
      }
    } else {
      result = { ok: false, error: String(e) }
    }
  }
  testing.value = false
  testResult.value = result

  // Sidebar tests have no edit form to show the result in — surface it in
  // the sidebar message area (this was the "no effect" bug).
  const label = target.name || target.host
  message.value = result.ok
    ? `✓ ${label}: ${result.message || '连接成功'}`
    : `✗ ${label}: ${result.error}`
}

async function doImport() {
  if (!importPath.value) return
  try {
    const n = await ImportFromRDM(importPath.value.trim())
    message.value = t('app.imported', { n })
    await reload()
  } catch (e) {
    message.value = String(e)
  }
}

// --- sidebar tree ------------------------------------------------------------

// OpenConnection with the askForSSHPassword retry loop: the backend returns
// "ssh password required" and we prompt, fill it in and retry (P6-5).
async function openConnWithRetry(cfg) {
  try {
    return await OpenConnection(cfg)
  } catch (e) {
    if (String(e).includes('ssh password required')) {
      const pw = prompt(`SSH 密码（${cfg.sshHost || cfg.ssh_host || ''}）:`)
      if (pw === null) throw e
      return await OpenConnection({ ...cfg, sshPassword: pw })
    }
    throw e
  }
}

async function toggleConn(cfg) {
  const next = !expandedConns.value[cfg.id]
  expandedConns.value = { ...expandedConns.value, [cfg.id]: next }
  if (next && !connMeta.value[cfg.id]) {
    opening.value = true
    try {
      // Full connect; db list (all dbs incl. empty) comes back in summary.
      connMeta.value = { ...connMeta.value, [cfg.id]: await openConnWithRetry(cfg) }
    } catch (e) {
      message.value = String(e)
    }
    opening.value = false
  }
}

function isActiveDb(connID, dbIndex) {
  return viewTab.value === 'browse' && opened.value?.id === connID && activeDb.value === dbIndex && !selectedKey.value
}

async function openDb(cfg, dbIndex) {
  opening.value = true
  let meta
  try {
    // Always route through EnsureConnection: reuses the live manager entry
    // or reconnects — stale caches can't produce "connection is not open".
    meta = await openConnWithRetryEnsure(cfg)
    connMeta.value = { ...connMeta.value, [cfg.id]: meta }
  } catch (e) {
    message.value = String(e)
    opening.value = false
    return
  }
  opening.value = false
  opened.value = meta
  activeDb.value = dbIndex
  selectedKey.value = null
  viewTab.value = 'browse'
  await loadTree()
}

// EnsureConnection wrapped with the SSH-password retry loop.
async function openConnWithRetryEnsure(cfg) {
  try {
    return await EnsureConnection(cfg)
  } catch (e) {
    if (String(e).includes('ssh password required')) {
      const pw = prompt(`SSH 密码（${cfg.sshHost || cfg.ssh_host || ''}）:`)
      if (pw === null) throw e
      return await EnsureConnection({ ...cfg, sshPassword: pw })
    }
    throw e
  }
}

// --- browse ------------------------------------------------------------------

async function closeConn() {
  if (opened.value) Disconnect(opened.value.id)
  opened.value = null
  tree.value = null
  selectedKey.value = null
}

const filterHistory = ref([])

async function loadTree() {
  if (!opened.value) return
  loadingTree.value = true
  tree.value = null // avoid rendering a stale tree from another db while loading
  try {
    tree.value = await RefreshKeys(opened.value.id, activeDb.value, filter.value || '*')
  } catch (e) {
    if (String(e).includes('is not open')) {
      // self-heal: reconnect via the stored config, then retry once
      const cfg = connections.value.find(c => c.id === opened.value.id)
      if (cfg) {
        try {
          opened.value = await openConnWithRetryEnsure(cfg)
          tree.value = await RefreshKeys(opened.value.id, activeDb.value, filter.value || '*')
        } catch (e2) {
          message.value = String(e2)
        }
      } else {
        message.value = String(e)
      }
    } else {
      message.value = String(e)
    }
  }
  if (opened.value && tree.value) {
    if (filter.value && filter.value !== '*') {
      try { await SaveFilterHistory(opened.value.id, activeDb.value, filter.value) } catch { /* non-fatal */ }
    }
    try { filterHistory.value = (await GetFilterHistory(opened.value.id, activeDb.value)) || [] } catch { /* */ }
  }
  loadingTree.value = false
}

function openKey(node) {
  selectedKey.value = node
}

// Reconnect the currently-opened connection (KeyView self-heal on
// "connection is not open"). Throws when the config is gone.
async function reopenConn() {
  const cfg = connections.value.find(c => c.id === opened.value?.id)
  if (!cfg) throw new Error('连接配置不存在，请重新展开连接')
  const meta = await openConnWithRetryEnsure(cfg)
  opened.value = meta
  connMeta.value = { ...connMeta.value, [cfg.id]: meta }
  return meta
}

async function onKeyChanged() {
  await loadTree()
}

onMounted(async () => {
  window.addEventListener('keydown', onGlobalKey)
  await reload()
  await loadPrefs()
})
</script>

<template>
  <div class="layout">
    <aside class="panel">
      <div class="panel-head">
        <b>{{ $t('app.connections') }}</b>
        <span class="head-actions">
          <button class="sm" @click="cycleLocale" :title="localeSetting">{{ localeSetting === 'zh_CN' ? '中' : 'EN' }}</button>
          <button class="sm" @click="toggleDark">{{ darkMode ? '☀' : '🌙' }}</button>
          <button class="sm" @click="showSettings = true" title="设置">⚙</button>
          <button class="sm primary" @click="edit(null)">{{ $t('app.newConnection') }}</button>
        </span>
      </div>
      <ul>
        <template v-for="c in connections" :key="c.id">
          <li>
            <span class="caret" @click="toggleConn(c)">{{ expandedConns[c.id] ? '▾' : '▸' }}</span>
            <span class="dot" :style="{ background: c.iconColor || '#d82c20' }"></span>
            <span class="name link" @click="toggleConn(c)">{{ c.name || c.host }}</span>
            <span class="meta">{{ c.host }}:{{ c.port }}</span>
            <span class="actions">
              <button class="sm" @click="test(c)" :disabled="testing">{{ $t('app.test') }}</button>
              <button class="sm" @click="edit(c)">{{ $t('app.edit') }}</button>
              <button class="sm danger" @click="askDeleteConn(c)">{{ $t('app.delete') }}</button>
            </span>
          </li>
          <template v-if="expandedConns[c.id]">
            <li v-if="!connMeta[c.id]" class="dbrow loading">
              <span class="spacer"></span>{{ opening ? '连接中…' : '未连接（点击名称重试）' }}
            </li>
            <li v-for="db in connMeta[c.id]?.databases || []" :key="c.id + ':' + db.index"
                class="dbrow" :class="{ active: isActiveDb(c.id, db.index) }"
                @click="openDb(c, db.index)">
              <span class="spacer"></span>
              <span class="db-icon">▦</span>
              <span class="name">db{{ db.index }}</span>
              <span class="meta">({{ db.keys }})</span>
            </li>
          </template>
        </template>
        <li v-if="!connections.length" class="empty">{{ $t('app.noConnections') }}</li>
      </ul>
      <div class="import">
        <input v-model="importPath" :placeholder="$t('app.importPlaceholder')" />
        <button @click="doImport">{{ $t('app.import') }}</button>
      </div>
      <p v-if="message" :class="['msg', message.startsWith('✗') ? 'msg-fail' : 'msg-ok']">{{ message }}</p>
    </aside>

    <main class="panel" v-if="editing">
      <h3>{{ editing.id ? $t('editor.editTitle') : $t('editor.newTitle') }}</h3>
      <div class="grid">
        <label>{{ $t('editor.name') }}<input v-model="editing.name" /></label>
        <label>{{ $t('editor.host') }}<input v-model="editing.host" /></label>
        <label>{{ $t('editor.port') }}<input v-model.number="editing.port" type="number" /></label>
        <label>{{ $t('editor.username') }}<input v-model="editing.username" /></label>
        <label>{{ $t('editor.password') }}<input v-model="editing.auth" type="password" /></label>
      </div>

      <details :open="editing.useSSL">
        <summary><input type="checkbox" v-model="editing.useSSL" @click.stop /> {{ $t('editor.ssl') }}</summary>
        <div class="grid">
          <label>{{ $t('editor.sslCA') }}<input v-model="editing.sslCACertPath" /></label>
          <label>{{ $t('editor.sslCert') }}<input v-model="editing.sslLocalCertPath" /></label>
          <label>{{ $t('editor.sslKey') }}<input v-model="editing.sslPrivateKeyPath" /></label>
          <label class="check"><input type="checkbox" v-model="editing.sslIgnoreAllErrors" /> {{ $t('editor.sslIgnore') }}</label>
        </div>
      </details>

      <details :open="editing.useSSHTunnel">
        <summary><input type="checkbox" v-model="editing.useSSHTunnel" @click.stop /> {{ $t('editor.ssh') }}</summary>
        <div class="grid">
          <label>{{ $t('editor.sshHost') }}<input v-model="editing.sshHost" /></label>
          <label>{{ $t('editor.sshPort') }}<input v-model.number="editing.sshPort" type="number" /></label>
          <label>{{ $t('editor.sshUser') }}<input v-model="editing.sshUser" /></label>
          <label>{{ $t('editor.sshPassword') }}<input v-model="editing.sshPassword" type="password" /></label>
          <label>{{ $t('editor.sshKey') }}<input v-model="editing.sshPrivateKeyPath" /></label>
          <label class="check"><input type="checkbox" v-model="editing.sshAgent" /> {{ $t('editor.sshAgent') }}</label>
          <label class="check"><input type="checkbox" v-model="editing.askForSSHPassword" /> {{ $t('editor.sshAsk') }}</label>
        </div>
      </details>

      <div class="btn-row">
        <button class="primary" @click="save">{{ $t('editor.save') }}</button>
        <button @click="test()" :disabled="testing">{{ testing ? $t('editor.testing') : $t('editor.testBtn') }}</button>
        <button @click="editing = null">{{ $t('editor.cancel') }}</button>
        <span v-if="testResult" :class="testResult.ok ? 'ok' : 'fail'">
          {{ testResult.ok ? `✓ ${testResult.message || 'OK'}` : `✗ ${testResult.error}` }}
        </span>
      </div>
    </main>

    <main class="panel browse" v-else-if="opened">
      <div class="conn-head">
        <div>
          <b>{{ opened.address }}</b>
          <span class="mode-badge">{{ opened.mode }}</span>
          <span class="meta">redis {{ opened.version }}</span>
        </div>
        <div class="view-tabs">
          <button v-for="tab in ['browse','console','server']" :key="tab"
                  :class="{ active: viewTab === tab }"
                  @click="viewTab = tab; selectedKey = null">
            {{ $t('tabs.' + tab) }}
          </button>
          <button @click="closeConn">{{ $t('browse.disconnect') }}</button>
        </div>
      </div>

      <template v-if="viewTab === 'console'">
        <ConsoleView :conn-id="opened.id" :db="activeDb"
                     :conn-cfg="connections.find(c => c.id === opened.id) || null"
                     :retry-conn="reopenConn"
                     @close="viewTab = 'browse'" />
      </template>
      <template v-else-if="viewTab === 'server'">
        <ServerView :conn-id="opened.id" :db="activeDb"
                    :conn-cfg="connections.find(c => c.id === opened.id) || null"
                    :retry-conn="reopenConn" />
      </template>
      <template v-else>
        <!-- tree stays mounted (v-show) so expansion state survives opening a key -->
        <div class="db-chips" v-show="!selectedKey">
          <button v-for="db in opened.databases" :key="db.index"
                  :class="['chip', { active: db.index === activeDb }]"
                  @click="activeDb = db.index; loadTree()">
            db{{ db.index }} <span class="count">{{ db.keys }}</span>
          </button>
        </div>

        <div class="toolbar" v-show="!selectedKey">
          <input v-model="filter" :placeholder="$t('browse.filterPlaceholder')" @keyup.enter="loadTree"
                 list="filter-history" />
          <datalist id="filter-history">
            <option v-for="p in filterHistory" :key="p" :value="p"></option>
          </datalist>
          <button class="primary" @click="loadTree" :disabled="loadingTree">
            {{ loadingTree ? $t('browse.scanning') : $t('browse.load') }}
          </button>
          <button @click="showBulk = true">{{ $t('browse.bulk') }}</button>
        </div>

        <div class="tree-area" v-show="!selectedKey">
          <template v-if="tree">
            <div class="tree-meta">
              {{ $t('browse.keys', { n: tree.total }) }}
              <span v-if="!tree.complete">{{ $t('browse.scanLimit') }}</span>
              <span class="flex1"></span>
              <button class="sm" @click="loadTree" :disabled="loadingTree" title="重新扫描键空间">🔄 刷新</button>
            </div>
            <div v-if="loadingTree" class="tree-loading">扫描中…</div>
            <VirtualTree :nodes="tree.nodes" @open="openKey" />
            <p v-if="!tree.nodes?.length && !loadingTree" class="empty-tree">{{ $t('browse.noMatch') }}</p>
          </template>
          <p v-else class="empty-tree">{{ loadingTree ? $t('browse.scanning') : $t('browse.loadHint') }}</p>
        </div>

        <KeyView v-if="selectedKey" :conn-id="opened.id" :db="activeDb" :node="selectedKey"
                 :conn-cfg="connections.find(c => c.id === opened.id) || null"
                 :retry-conn="reopenConn"
                 @close="selectedKey = null" @changed="onKeyChanged" />
      </template>

      <BulkDialog v-if="showBulk" :conn-id="opened.id" :db="activeDb" :connections="connections"
                  @close="showBulk = false" @changed="onKeyChanged" />
    </main>

    <main class="panel placeholder" v-if="!editing && !opened">
      <p>{{ $t('app.openHint') }}</p>
      <p class="hint">{{ $t('app.p1Hint') }}</p>
    </main>

    <SettingsDialog v-if="showSettings" @close="showSettings = false; loadPrefs()" />

    <!-- connection delete confirm -->
    <div class="modal-mask" v-if="pendingDel" @click.self="pendingDel = null">
      <div class="modal ask">
        <p class="ask-msg">删除连接「{{ pendingDel.name || pendingDel.host }}」？该操作不可恢复。</p>
        <div class="btn-row">
          <button class="primary danger" @click="doDeleteConn">删除</button>
          <button @click="pendingDel = null">取消</button>
        </div>
      </div>
    </div>

    <!-- toasts -->
    <div class="toasts">
      <div v-for="t in toasts" :key="t.id" :class="['toast', t.type]">{{ t.msg }}</div>
    </div>
  </div>
</template>

<style scoped>
.layout { display: flex; gap: 16px; padding: 16px; height: 100vh; }
.panel { background: var(--panel); border-radius: 10px; padding: 16px; box-shadow: 0 1px 4px rgba(0,0,0,.08); color: var(--fg); }
aside { width: 380px; flex-shrink: 0; display: flex; flex-direction: column; }
main { flex: 1; overflow: auto; display: flex; flex-direction: column; gap: 10px; }
.placeholder { align-items: center; justify-content: center; color: var(--muted); }
.hint { font-size: 12px; color: var(--muted); }
.panel-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.head-actions { display: flex; gap: 6px; }
ul { list-style: none; margin: 0; padding: 0; flex: 1; overflow: auto; }
li { display: flex; align-items: center; gap: 8px; padding: 6px 4px; border-bottom: 1px solid var(--border); }
li.empty { color: var(--muted); justify-content: center; border: none; }
.caret { width: 12px; cursor: pointer; color: var(--muted); user-select: none; flex-shrink: 0; }
.dbrow { cursor: pointer; padding-left: 12px; }
.dbrow:hover { background: rgba(128,128,128,.1); }
.dbrow.active { background: rgba(53,103,179,.18); }
.dbrow.loading { color: var(--muted); cursor: default; }
.dbrow .spacer { width: 12px; flex-shrink: 0; }
.db-icon { color: #d82c20; font-size: 12px; }
.dbrow .meta { flex: 0 0 auto; }
.dot { width: 10px; height: 10px; border-radius: 50%; }
.name { font-weight: 500; }
.link { cursor: pointer; }
.link:hover { text-decoration: underline; }
.meta { color: var(--muted); font-size: 12px; flex: 1; }
.actions { display: flex; gap: 4px; }
.import { display: flex; gap: 6px; margin-top: 10px; }
.import input { flex: 1; }
.msg { font-size: 12px; word-break: break-all; }
.msg-ok { color: #2a7; }
.msg-fail { color: #d82c20; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 16px; margin: 10px 0; }
label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
label.check { flex-direction: row; align-items: center; font-size: 13px; color: inherit; }
input, select { padding: 6px 8px; border: 1px solid var(--border); border-radius: 6px; background: var(--panel); color: var(--fg); }
details { border: 1px solid var(--border); border-radius: 8px; padding: 8px 12px; margin: 8px 0; }
summary { cursor: pointer; font-weight: 500; user-select: none; }
.btn-row { display: flex; align-items: center; gap: 10px; margin-top: 12px; }
button { padding: 6px 12px; border: 1px solid var(--border); border-radius: 6px; background: transparent; cursor: pointer; color: var(--fg); }
button.sm { padding: 3px 9px; font-size: 12px; }
button.primary { background: #d82c20; border-color: #d82c20; color: #fff; }
button.danger { color: #d82c20; }
button:hover { filter: brightness(.95); }
button:disabled { opacity: .5; cursor: wait; }
.ok { color: #2a7; } .fail { color: #d82c20; }

.conn-head { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; }
.mode-badge { background: rgba(53,103,179,.15); color: #3567b3; font-size: 11px; padding: 2px 8px; border-radius: 8px; margin-left: 8px; }
.view-tabs { display: flex; gap: 6px; }
.view-tabs button.active { background: #3567b3; border-color: #3567b3; color: #fff; }
.db-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.chip { font-size: 12px; padding: 3px 10px; border-radius: 12px; }
.chip.active { background: #3567b3; border-color: #3567b3; color: #fff; }
.chip .count { opacity: .7; margin-left: 3px; }
.toolbar { display: flex; gap: 8px; }
.toolbar input { flex: 1; font-family: 'SF Mono', Menlo, monospace; }
.tree-area { flex: 1; overflow: auto; border: 1px solid var(--border); border-radius: 8px; padding: 8px; display: flex; flex-direction: column; }
.tree-meta { font-size: 12px; color: var(--muted); margin-bottom: 6px; display: flex; align-items: center; gap: 6px; }
.tree-meta .sm { padding: 1px 8px; font-size: 11px; }
.empty-tree { color: var(--muted); text-align: center; margin-top: 40px; }
.tree-loading { color: var(--muted); font-size: 12px; padding: 4px 2px; }
.modal-mask { position: fixed; inset: 0; background: rgba(0,0,0,.4); display: flex; align-items: center; justify-content: center; z-index: 25; }
.modal.ask { background: var(--panel); color: var(--fg); border-radius: 12px; padding: 20px; width: min(420px, 80vw); }
.ask-msg { word-break: break-all; font-size: 13px; margin: 4px 0 14px; }
.btn-row { display: flex; gap: 10px; }
button.danger { color: #d82c20; }
button.primary.danger { background: #d82c20; border-color: #d82c20; color: #fff; }
.toasts { position: fixed; right: 20px; bottom: 20px; display: flex; flex-direction: column; gap: 8px; z-index: 40; }
.toast { background: #2a7; color: #fff; padding: 9px 16px; border-radius: 8px; font-size: 13px;
         box-shadow: 0 2px 8px rgba(0,0,0,.2); animation: toast-in .15s ease-out; max-width: 420px; word-break: break-all; }
.toast.err { background: #d82c20; }
@keyframes toast-in { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: none; } }
</style>
