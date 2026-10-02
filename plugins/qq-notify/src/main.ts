import { createApp, defineComponent, h, reactive } from 'vue'
import { NConfigProvider, NDialogProvider, NMessageProvider, NNotificationProvider } from 'naive-ui'
import AppPage from './AppPage.vue'
import './preview.css'
const previewState = reactive<any>({config:{url:'http://napcat:3000',credential_ref:'',target_type:'private',target_id:'123456789',enabled:false,prefix:'[DIAN115]',template:'{prefix} {title}\n{body}\n{time}',levels:['success','failed','warning','info'],interval_seconds:60,max_attempts:3,emby_enabled:false,proxy_id:null,library_id:'',media_types:['Movie','Episode'],adapters:[]},logs:[],pending:0,dead:0,delivered:0,last_poll:0,last_error:''})
const previewBridge = {
  async getState(){return {state:previewState}},
  async refresh(){return previewState},
  async invokeAction(action:string,input?:unknown){
    if(action==='save-config') previewState.config=JSON.parse(JSON.stringify(input))
    let data:any
    if(action==='instances') data={items:[{id:1,name:'家庭媒体库',api_key_configured:true}]}
    if(action==='libraries') data={items:[{id:'library-1',name:'电影'}]}
    return {result:{status:'succeeded',message:'预览模拟操作：'+action,data,sample:action==='diagnose' ? '{"items":[]}' : undefined}}
  },
}
const Preview=defineComponent({setup:()=>()=>h(NConfigProvider,null,{default:()=>h(NMessageProvider,null,{default:()=>h(NNotificationProvider,null,{default:()=>h(NDialogProvider,null,{default:()=>h(AppPage,{api:previewBridge,installationId:undefined,runtimeState:previewState,themeContract:'dian115-theme-v1'})})})})})})
createApp(Preview).mount('#app')
