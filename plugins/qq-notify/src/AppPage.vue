<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { NAlert, NButton, NCheckbox, NCheckboxGroup, NEmpty, NForm, NFormItem, NIcon, NInput, NInputNumber, NSelect, NSpace, NSwitch, NTag, useMessage } from 'naive-ui'
import { Bell, RefreshCw, Save, ShieldCheck } from '@lucide/vue'
interface Config {
  url: string; credential_ref: string; target_type: string; target_id: string; enabled: boolean;
  prefix: string; template: string; levels: string[]; interval_seconds: number; max_attempts: number;
  emby_enabled: boolean; proxy_id: number | null; library_id: string; media_types: string[]; adapters: unknown[];
}
interface Log { at: string; title: string; status: string; message: string }
interface Snapshot { adapter_routes_relative?: boolean; config?: Config; logs?: Log[]; pending?: number; dead?: number; delivered?: number; last_poll?: number; last_error?: string }
interface Callback { result?: {status: string; message?: string; data?: {items?: any[]}; bot?: {nickname?: string; user_id?: string}; sample?: string} }
const props = defineProps<{
  api: {getState(view?: string): Promise<{state?: Snapshot}>; invokeAction(action: string, input?: unknown): Promise<Callback>; refresh(): Promise<Snapshot>};
  installationId?: number; runtimeState?: Snapshot; themeContract?: string;
}>()
const message = useMessage()
const busy = ref(''), loadError = ref(''), loaded = ref(false), token = ref(''), credentialBusy = ref(false)
const snapshot = ref<Snapshot>({})
const config = reactive<Config>({url:'http://napcat:3000',credential_ref:'',target_type:'private',target_id:'',enabled:false,prefix:'[DIAN115]',template:'{prefix} {title}\n{body}\n{time}',levels:['success','failed','warning','info'],interval_seconds:60,max_attempts:3,emby_enabled:false,proxy_id:null,library_id:'',media_types:['Movie','Episode'],adapters:[]})
const adaptersJSON = ref('[]'), sample = ref(''), diagnosePath = ref('/api/organize/history')
const instances = ref<{label: string; value: number}[]>([]), libraries = ref<{label: string; value: string}[]>([])
const current = computed(() => props.runtimeState?.config ? props.runtimeState : snapshot.value)
const logs = computed(() => current.value.logs || [])
const lastPoll = computed(() => current.value.last_poll ? new Date(current.value.last_poll * 1000).toLocaleString() : '等待首次检测')
async function load() {
  busy.value = 'load'; loadError.value = ''
  try {
    const s = (await props.api.getState('main')).state || {}
    snapshot.value = s
    if (s.config) {
      Object.assign(config, JSON.parse(JSON.stringify(s.config)))
      if (s.adapter_routes_relative) config.adapters = config.adapters.map((value: any) => { const a = {...value}; for (const key of ['path','list_pointer','id_pointer','status_pointer','title_pointer','body_pointer']) if (a[key]) a[key] = '/' + a[key]; return a })
    }
    adaptersJSON.value = JSON.stringify(config.adapters || [], null, 2); loaded.value = true
  } catch (e: any) { loadError.value = e.message || '读取配置失败' }
  finally { busy.value = '' }
}
onMounted(load)
async function run(action: string, input: unknown = {}) {
  busy.value = action
  try {
    const wrapper = await props.api.invokeAction(action, input), result = wrapper.result
    if (!result || result.status === 'failed') throw new Error(result?.message || '动作失败')
    snapshot.value = await props.api.refresh()
    if (result.status === 'accepted') message.info(result.message || '后台请求已接受')
    else message.success(result.message || '已完成')
    return result
  } catch (e: any) { message.error(e.message || '操作失败'); return undefined }
  finally { busy.value = '' }
}
async function refreshStatus() {
  busy.value = 'refresh'
  try { snapshot.value = await props.api.refresh() } catch (e: any) { message.error(e.message || '刷新失败') } finally { busy.value = '' }
}
async function save() {
  try {
    const parsed = JSON.parse(adaptersJSON.value)
    if (!Array.isArray(parsed)) throw new Error('历史适配器必须为 JSON 数组')
    const input = JSON.parse(JSON.stringify({...config, adapters: parsed}))
    const response = await run('save-config', input)
    if (response) config.adapters = parsed
  } catch (e: any) { message.error(e.message || '配置无效') }
}
async function loadInstances() {
  const result = await run('instances')
  if (result?.data?.items) instances.value = result.data.items.map(i => ({label:`${i.name} (ID ${i.id})${i.api_key_configured ? '' : ' · 未配置凭据'}`,value:i.id}))
}
async function loadLibraries() {
  const result = await run('libraries')
  if (result?.data?.items) libraries.value = [{label:'全部媒体库',value:''},...result.data.items.map(i => ({label:i.name,value:i.id}))]
}
async function diagnose() {
  const result = await run('diagnose', {path:diagnosePath.value})
  sample.value = result?.sample || ''
}
async function createCredential() {
  if (!token.value || !props.installationId) { message.warning('请输入 Token，并在已安装的插件页面中操作'); return }
  credentialBusy.value = true
  try {
    const url = new URL(config.url)
    if (!['http:','https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || !['','/'].includes(url.pathname)) throw new Error('请填写有效 NapCat origin')
    const response = await fetch(`/api/plugin-center/v1/installations/${props.installationId}/secret-bindings`, {
      method:'POST', credentials:'same-origin', headers:{'content-type':'application/json','accept':'application/json'},
      body:JSON.stringify({label:'NapCat OneBot Token',host:url.hostname,method:'POST',path_prefix:'/',injection_mode:'static',location:'header',name:'authorization',prefix:'Bearer ',suffix:'',secret:token.value}),
    })
    if (!response.ok) throw new Error(`宿主管理端未接受凭据（HTTP ${response.status}）。请在插件管理的托管凭据中创建，再填写 credential_ref。`)
    const data = await response.json()
    const ref = data.credential_ref || data.data?.credential_ref || data.binding?.credential_ref
    if (typeof ref !== 'string' || !ref.startsWith('cred_')) throw new Error('宿主返回未含 credential_ref。请在管理端核对绑定，避免重复创建。')
    config.credential_ref = ref
    message.success('Token 已交给宿主保存，请点击「保存配置」')
  } catch (e: any) { message.error(e.message || '创建托管凭据失败') }
  finally { token.value = ''; credentialBusy.value = false }
}
</script>

<template>
  <main class="dian-plugin-page qq-page">
    <header class="header"><div><h2>QQ 通知</h2><p>将 DIAN115 的新增媒体和已接入事件发送到 QQ。</p></div><NTag :type="current.config?.enabled ? 'success' : 'default'">{{ current.config?.enabled ? '自动通知已开启' : '自动通知已暂停' }}</NTag></header>
    <NAlert v-if="loadError" type="error" title="读取失败">{{ loadError }} <NButton @click="load">重试</NButton></NAlert>
    <NAlert type="info" :bordered="false">先保存配置，再测试连接和消息。新来源首次检测只建立基线；关闭自动通知会暂停检测与队列发送。</NAlert>
    <NAlert v-if="current.last_error" type="warning" :bordered="false">{{ current.last_error }}</NAlert>
    <section class="summary"><span>已发送 <strong>{{ current.delivered || 0 }}</strong></span><span>待发送 <strong>{{ current.pending || 0 }}</strong></span><span>需处理 <strong>{{ current.dead || 0 }}</strong></span><span>最近检测 {{ lastPoll }}</span></section>
    <NForm :disabled="!loaded || !!busy || credentialBusy" label-placement="top" @submit.prevent="save">
      <section class="section"><h3>机器人连接与通知目标</h3><div class="form-grid">
        <NFormItem label="NapCat HTTP API 地址"><NInput v-model:value="config.url" placeholder="http://你的NAS_IP:3000" /></NFormItem>
        <NFormItem label="托管凭据引用 credential_ref"><NInput v-model:value="config.credential_ref" placeholder="cred_…，未开启鉴权时可留空" /></NFormItem>
        <NFormItem label="发送方式"><NSelect v-model:value="config.target_type" :options="[{label:'QQ 私聊',value:'private'},{label:'QQ 群',value:'group'}]" /></NFormItem>
        <NFormItem :label="config.target_type === 'group' ? '群号（机器人必须已加入）' : 'QQ 号（建议先添加机器人好友）'"><NInput v-model:value="config.target_id" placeholder="数字，不填写昵称" /></NFormItem>
      </div><details><summary>安全保存 NapCat Access Token</summary><p>Token 只提交给 DIAN115 的托管凭据接口，不进入插件后端、配置、浏览器存储或日志。内网 HTTP 会以明文传输 Token。</p><NSpace align="center"><NInput v-model:value="token" type="password" show-password-on="click" placeholder="NapCat HTTP 服务的 Access Token" autocomplete="new-password" /><NButton :loading="credentialBusy" :disabled="!!busy || !token" @click="createCredential"><template #icon><NIcon :component="ShieldCheck" /></template>交给宿主保存</NButton></NSpace><p>若宿主管理接口需要额外登录验证，请在插件管理端创建静态 Header 凭据：host 为 NapCat hostname，method 为 POST，path_prefix 为 /，name 为 authorization，prefix 为 Bearer 加一个空格。</p></details></section>
      <section class="section"><h3>自动通知</h3><NFormItem label="启用自动检测及队列发送"><NSwitch v-model:value="config.enabled" /></NFormItem><div class="form-grid">
        <NFormItem label="轮询间隔（秒）"><NInputNumber v-model:value="config.interval_seconds" :min="30" :max="3600" /></NFormItem><NFormItem label="总尝试次数（包含第一次）"><NInputNumber v-model:value="config.max_attempts" :min="1" :max="5" /></NFormItem>
        <NFormItem label="消息前缀"><NInput v-model:value="config.prefix" /></NFormItem><NFormItem label="通知级别"><NCheckboxGroup v-model:value="config.levels"><NSpace><NCheckbox value="success">成功</NCheckbox><NCheckbox value="failed">失败</NCheckbox><NCheckbox value="warning">警告</NCheckbox><NCheckbox value="info">信息</NCheckbox></NSpace></NCheckboxGroup></NFormItem>
      </div><NFormItem label="消息模板：{prefix}、{title}、{body}、{level}、{time}"><NInput v-model:value="config.template" type="textarea" :autosize="{minRows:3,maxRows:8}" /></NFormItem></section>
      <section class="section"><h3>Emby 新入库</h3><p>通过宿主已有 Emby 配置读取，支持电影和剧集。无需提供 Emby API Key。</p><NFormItem label="启用新入库检测"><NSwitch v-model:value="config.emby_enabled" /></NFormItem><div class="form-grid">
        <NFormItem label="Emby 实例"><NSpace vertical><NSelect v-model:value="config.proxy_id" :options="instances" placeholder="先读取实例，再明确选择" /><NButton :loading="busy === 'instances'" @click="loadInstances">读取宿主实例</NButton></NSpace></NFormItem>
        <NFormItem label="媒体库（先保存实例选择再读取）"><NSpace vertical><NSelect v-model:value="config.library_id" :options="libraries" placeholder="留空检测全部媒体库" /><NButton :loading="busy === 'libraries'" @click="loadLibraries">读取媒体库</NButton></NSpace></NFormItem>
      </div><NFormItem label="媒体类型"><NCheckboxGroup v-model:value="config.media_types"><NSpace><NCheckbox value="Movie">电影</NCheckbox><NCheckbox value="Episode">剧集</NCheckbox></NSpace></NCheckboxGroup></NFormItem></section>
      <section class="section"><details><summary>高级：整理、后台任务和操作历史适配器</summary><NAlert type="warning" :bordered="false">这些接口的公开文档未定义具体返回字段。适配器默认为空，诊断真实响应后再配置映射。每次查询仅检测接口返回的窗口，不保证离线期间的全部历史；请勿填写未经验证的字段。</NAlert><NFormItem label="适配器 JSON 数组"><NInput v-model:value="adaptersJSON" type="textarea" :autosize="{minRows:4,maxRows:15}" /></NFormItem><p>格式和 JSON Pointer 映射说明见交付包 README。状态映射只填写需要发送的终态；未知状态作为进行中处理。</p><NSpace><NSelect v-model:value="diagnosePath" :options="[{label:'整理历史',value:'/api/organize/history'},{label:'后台任务',value:'/api/tasks'},{label:'操作历史',value:'/api/history'}]" /><NButton :loading="busy === 'diagnose'" @click="diagnose">读取脱敏样本</NButton></NSpace><pre v-if="sample">{{ sample }}</pre></details></section>
      <NSpace class="actions"><NButton type="primary" attr-type="submit" :loading="busy === 'save-config'" :disabled="!loaded || !!busy || credentialBusy"><template #icon><NIcon :component="Save" /></template>保存配置</NButton></NSpace>
    </NForm>
    <NSpace class="actions"><NButton :disabled="!loaded || !!busy || credentialBusy" :loading="busy === 'test-connection'" @click="run('test-connection')">测试连接</NButton><NButton :disabled="!loaded || !!busy || credentialBusy" :loading="busy === 'send-test'" @click="run('send-test')"><template #icon><NIcon :component="Bell" /></template>发送测试消息</NButton><NButton :disabled="!loaded || !!busy" :loading="busy === 'poll-now'" @click="run('poll-now')">立即检测</NButton><NButton :disabled="!loaded || !!busy || !(current.dead || current.pending)" :loading="busy === 'retry-now'" @click="run('retry-now')">用当前配置重试失败队列</NButton><NButton :disabled="!!busy" @click="refreshStatus"><template #icon><NIcon :component="RefreshCw" /></template>刷新状态</NButton></NSpace>
    <section class="section"><h3>最近发送与检测日志</h3><NEmpty v-if="!logs.length" description="尚无后台通知记录。测试消息的结果显示在操作提示中。" /><div v-else class="table-scroll"><table><thead><tr><th>时间</th><th>事件</th><th>结果</th><th>详情</th></tr></thead><tbody><tr v-for="(entry,i) in logs" :key="entry.at + i"><td>{{ new Date(entry.at).toLocaleString() }}</td><td>{{ entry.title }}</td><td><NTag :type="entry.status === 'succeeded' ? 'success' : 'error'" size="small">{{ entry.status === 'succeeded' ? '已发送' : '需检查' }}</NTag></td><td>{{ entry.message }}</td></tr></tbody></table></div></section>
  </main>
</template>
<style scoped>
.qq-page{display:grid;gap:var(--dian-space-4);padding:var(--dian-space-2)}
.header{display:flex;align-items:center;justify-content:space-between;gap:var(--dian-space-3)}
h2,h3,p{margin:0}h2{font-size:22px}h3{font-size:16px;margin-bottom:var(--dian-space-4)}p{margin:var(--dian-space-2) 0;color:var(--dian-text-secondary);overflow-wrap:anywhere}
.section{padding:var(--dian-space-4);border-top:1px solid var(--dian-divider)}
.form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));column-gap:var(--dian-space-5)}
.summary{display:flex;flex-wrap:wrap;gap:var(--dian-space-4);padding:var(--dian-space-4);background:var(--dian-surface-soft);border-radius:var(--dian-radius-md)}
summary{cursor:pointer;font-weight:600;margin-bottom:var(--dian-space-4)}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:12px var(--dian-font-mono);max-height:420px;overflow:auto;background:var(--dian-surface-soft);padding:var(--dian-space-3)}
.actions{padding:var(--dian-space-2) var(--dian-space-4)}.table-scroll{overflow:auto}table{width:100%;border-collapse:collapse;font-size:13px}th,td{text-align:left;padding:var(--dian-space-3);border-bottom:1px solid var(--dian-divider);overflow-wrap:anywhere}th{color:var(--dian-text-secondary)}td:first-child{min-width:130px}
@media(max-width:900px){.form-grid{grid-template-columns:minmax(0,1fr)}.header{align-items:flex-start;flex-direction:column}}
</style>
