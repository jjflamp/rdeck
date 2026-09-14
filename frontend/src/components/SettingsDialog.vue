<script setup>
// Global settings dialog (P6): locale, limits, Extension Server URL.
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { applyLocale } from '../i18n'

const { locale } = useI18n()
const A = window.go.app.App
const emit = defineEmits(['close'])

const s = ref({
  locale: 'system',
  value_size_limit: 150000,
  scan_limit: 10000,
  ext_server_url: '',
  dark_mode: false,
})
const saved = ref(false)
const extTest = ref('')

onMounted(async () => {
  s.value = { ...s.value, ...(await A.GetSettings()) }
})

async function save() {
  await A.SetSettings(s.value)
  applyLocale(s.value.locale)
  saved.value = true
  setTimeout(() => emit('close'), 400)
}

async function testExt() {
  extTest.value = '…'
  // Test by listing formatters through the decode path: reuse ListFormatters
  // server-side? Direct: save temp then list. Simplest: call a probe binding.
  try {
    const list = await A.ProbeExtServer(s.value.ext_server_url)
    extTest.value = `✓ ${list.length} 个 formatter`
  } catch (e) {
    extTest.value = '✗ ' + String(e)
  }
}
</script>

<template>
  <div class="modal-mask" @click.self="emit('close')">
    <div class="modal">
      <h4>设置</h4>

      <label>界面语言
        <select v-model="s.locale">
          <option value="system">跟随系统</option>
          <option value="zh_CN">中文</option>
          <option value="en">English</option>
        </select>
      </label>

      <label>单值大小上限（字节，超出截断显示）
        <input v-model.number="s.value_size_limit" type="number" />
      </label>

      <label>键空间扫描上限（每库）
        <input v-model.number="s.scan_limit" type="number" />
      </label>

      <label>Extension Server URL（外部格式化器，可选）
        <div class="row">
          <input v-model="s.ext_server_url" placeholder="http://localhost:8080" />
          <button @click="testExt">测试</button>
        </div>
        <span v-if="extTest" class="hint">{{ extTest }}</span>
      </label>

      <div class="btn-row">
        <button class="primary" @click="save">保存</button>
        <button @click="emit('close')">取消</button>
        <span v-if="saved" class="ok">✓</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-mask { position: fixed; inset: 0; background: rgba(0,0,0,.4); display: flex; align-items: center; justify-content: center; z-index: 30; }
.modal { background: var(--panel); color: var(--fg); border-radius: 12px; padding: 22px; width: min(480px, 90vw); display: flex; flex-direction: column; gap: 12px; }
h4 { margin: 0; }
label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
input, select { padding: 7px 9px; border: 1px solid var(--border); border-radius: 6px; background: var(--panel); color: var(--fg); }
.row { display: flex; gap: 6px; }
.row input { flex: 1; }
.hint { font-size: 12px; }
.btn-row { display: flex; gap: 10px; align-items: center; }
button { padding: 7px 14px; border: 1px solid var(--border); border-radius: 6px; background: transparent; cursor: pointer; color: var(--fg); }
button.primary { background: #d82c20; border-color: #d82c20; color: #fff; }
.ok { color: #2a7; }
</style>
