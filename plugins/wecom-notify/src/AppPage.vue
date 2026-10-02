<script setup lang="ts">
import {onMounted, reactive, ref} from 'vue'
import {NAlert,NButton,NFormItem,NInput,NInputNumber,NSelect,NSwitch,NTag,useMessage} from 'naive-ui'
const props=defineProps<{api:any;installationId?:number;runtimeState?:any}>()
const message=useMessage(), busy=ref(false), state=ref<any>({}), secret=ref(''), instances=ref<any[]>([])
const config=reactive<any>({url:'https://qyapi.weixin.qq.com',credential_ref:'',target_type:'private',target_id:'',corp_id:'',agent_id:1000002,enabled:false,prefix:'[DIAN115]',template:'{prefix} {title}\n{body}\n{time}',levels:['success','failed','warning','info'],interval_seconds:60,max_attempts:3,emby_enabled:true,proxy_id:null,library_id:'',media_types:['Movie','Episode'],adapters:[]})
async function load(){state.value=(await props.api.getState('main')).state||{};if(state.value.config)Object.assign(config,state.value.config)}
onMounted(async()=>{try{await load()}catch(e:any){message.error(e.message)}})
async function run(action:string,input:any={}){busy.value=true;try{const v=(await props.api.invokeAction(action,input)).result;if(!v||v.status==='failed')throw Error(v?.message||'操作失败');message.success(v.message||'已完成');await load();return v}catch(e:any){message.error(e.message)}finally{busy.value=false}}
async function bind(){if(!secret.value||!props.installationId)return;busy.value=true;try{const r=await fetch(`/api/plugin-center/v1/installations/${props.installationId}/secret-bindings`,{method:'POST',credentials:'same-origin',headers:{'content-type':'application/json'},body:JSON.stringify({label:'企业微信应用 Secret',host:'qyapi.weixin.qq.com',method:'GET',path_prefix:'/cgi-bin/gettoken',injection_mode:'static',location:'query',name:'corpsecret',prefix:'',suffix:'',secret:secret.value})});if(!r.ok)throw Error(`绑定失败 HTTP ${r.status}，可在插件管理的托管凭据中绑定`);const d=await r.json();const key=d.credential_ref||d.data?.credential_ref||d.binding?.credential_ref;if(!key?.startsWith('cred_'))throw Error('绑定响应缺少凭据引用，请在管理页核对');config.credential_ref=key;message.success('Secret 已交由宿主保存，请保存配置')}catch(e:any){message.error(e.message)}finally{secret.value='';busy.value=false}}
async function readInstances(){const v=await run('instances');instances.value=(v?.data?.items||[]).map((i:any)=>({label:i.name,value:i.id}))}
</script>
<template>
 <main class="page">
  <h2>企业微信通知</h2><NAlert type="info">向指定企业微信成员发送通知。首次检查建立基线，只通知之后的新入库；图文卡片可点击打开 DIAN115。</NAlert>
  <NAlert v-if="state.last_error" type="warning">{{state.last_error}}</NAlert>
  <section><h3>应用与接收人</h3>
   <NFormItem label="企业 ID（CorpID）"><NInput v-model:value="config.corp_id" /></NFormItem>
   <NFormItem label="应用 ID（AgentID）"><NInputNumber v-model:value="config.agent_id" :min="1" /></NFormItem>
   <NFormItem label="接收成员 UserID（成员账号，不是显示名称）"><NInput v-model:value="config.target_id" /></NFormItem>
   <NFormItem label="应用 Secret（绑定后立即清空）"><NInput v-model:value="secret" type="password" show-password-on="click" /></NFormItem>
   <NButton :disabled="busy||!secret" @click="bind">绑定 Secret</NButton>
   <NFormItem label="宿主凭据引用"><NInput v-model:value="config.credential_ref" placeholder="cred_…" /></NFormItem>
  </section>
  <section><h3>Emby 入库通知</h3>
   <NFormItem label="启用自动通知"><NSwitch v-model:value="config.enabled" /></NFormItem>
   <NFormItem label="电影、剧集入库通知"><NSwitch v-model:value="config.emby_enabled" /></NFormItem>
   <NFormItem label="Emby 实例"><NSelect v-model:value="config.proxy_id" :options="instances" /><NButton :disabled="busy" @click="readInstances">读取实例</NButton></NFormItem>
   <NFormItem label="媒体库 ID（留空为全部）"><NInput v-model:value="config.library_id" /></NFormItem>
   <NFormItem label="检查间隔（秒）"><NInputNumber v-model:value="config.interval_seconds" :min="30" :max="3600" /></NFormItem>
   <NFormItem label="通知模板"><NInput v-model:value="config.template" type="textarea" /></NFormItem>
  </section>
  <div class="actions"><NButton type="primary" :disabled="busy" @click="run('save-config',JSON.parse(JSON.stringify(config)))">保存配置</NButton><NButton :disabled="busy" @click="run('test-connection')">验证凭据</NButton><NButton :disabled="busy" @click="run('send-test')">发送入库样例</NButton><NButton :disabled="busy" @click="run('poll-now')">立即检查</NButton><NButton :disabled="busy" @click="run('retry-now')">重试失败队列</NButton></div>
  <p>已发送 {{state.delivered||0}} · 待处理 {{state.pending||0}} · 失败 {{state.dead||0}}</p>
  <section><h3>通知记录</h3><article v-for="(log,i) in state.logs||[]" :key="i"><NTag :type="log.status==='succeeded'?'success':'error'">{{log.status==='succeeded'?'已发送':'失败'}}</NTag> {{log.title}} · {{log.message}}</article></section>
 </main>
</template>
<style scoped>
.page{display:grid;gap:var(--dian-space-4);padding:var(--dian-space-4)}h2,h3{margin:0}section{padding:var(--dian-space-4);border-top:1px solid var(--dian-divider)}h3{margin-bottom:var(--dian-space-4)}.actions{display:flex;flex-wrap:wrap;gap:var(--dian-space-3)}p,article{color:var(--dian-text-secondary);overflow-wrap:anywhere}article{padding:var(--dian-space-3) 0}
</style>
