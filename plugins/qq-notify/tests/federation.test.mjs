import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createSSRApp, h } from 'vue'
import { renderToString } from '@vue/server-renderer'
import * as Vue from 'vue'
import * as Naive from 'naive-ui'
import * as Lucide from '@lucide/vue'
import { setup as setupCssSSR } from '@css-render/vue3-ssr'

test('production Federation module loads with host singletons and renders settings page',async()=>{
  const prior=globalThis.document
  const links=[]
  globalThis.document={createElement:()=>({}),head:{appendChild:x=>links.push(x)}}
  try {
    const remote=await import('../build/frontend/dist/assets/remoteEntry.js')
    remote.init({vue:{'3.5.0':{get:async()=>()=>Vue}},'naive-ui':{'2.44.0':{get:async()=>()=>Naive}},'@lucide/vue':{'1.16.0':{get:async()=>()=>Lucide}}})
    const component=(await remote.get('./AppPage'))()
    globalThis.document=prior
    const api={getState:async()=>({state:{}}),invokeAction:async()=>({result:{status:'succeeded'}}),refresh:async()=>({})}
    const app=createSSRApp({render:()=>h(Naive.NConfigProvider,null,{default:()=>h(Naive.NMessageProvider,null,{default:()=>h(component,{api})})})})
    setupCssSSR(app)
    app.config.warnHandler=()=>{}
    const html=await renderToString(app)
    for(const text of ['QQ 通知','机器人连接与通知目标','保存配置','测试连接','发送测试消息','Emby 新入库','最近发送与检测日志']) assert.ok(html.includes(text),text)
    assert.ok(links.some(x=>x.rel==='stylesheet'))
    const entry=readFileSync(new URL('../build/frontend/dist/assets/remoteEntry.js',import.meta.url),'utf8')
    assert.ok(!entry.includes('https://cdn'),'framework must be bundled via host singletons')
  } finally {globalThis.document=prior}
})
