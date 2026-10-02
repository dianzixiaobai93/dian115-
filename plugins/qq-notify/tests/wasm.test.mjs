import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { WASI } from 'node:wasi'

async function harness() {
  const wasi = new WASI({version:'preview1',args:[],env:{},preopens:{}})
  const store = new Map(), rev = new Map(), calls = []
  const idempotency = new Map()
  let instance, reply
  const decode = x => JSON.parse(Buffer.from(x,'base64').toString('utf8'))
  const response = (status,value,headers={}) => ({status,headers,body_base64:Buffer.from(JSON.stringify(value)).toString('base64').replace(/=+$/,'')})
  const imports = {
    wasi_snapshot_preview1: wasi.wasiImport,
    dian115: {
      host_call(ptr,length) {
        const request = JSON.parse(Buffer.from(instance.exports.memory.buffer,ptr,length).toString('utf8'))
        calls.push(request)
        let result
        if(request.method==='host.log') result = {}
        else {
          assert.equal(request.method,'host.call')
          const p=request.params,url=new URL(p.path,'http://host')
          if(url.pathname.startsWith('/api/plugin-runtime/storage/')) {
            const key=url.pathname.split('/').at(-1)
            if(p.method==='GET') result=store.has(key) ? response(200,{data:{value:store.get(key),revision:`pkv_${rev.get(key)}`},meta:{}},{etag:[`"pkv_${rev.get(key)}"`]}) : response(404,{})
            else {
              assert.equal(p.method,'PUT'); assert.match(p.headers['idempotency-key'],/^[\x20-\x7e]{16,128}$/)
              const id=p.headers['idempotency-key'], fingerprint=JSON.stringify([p.path,p.headers['if-match'],p.body_base64])
              if(idempotency.has(id)) assert.equal(idempotency.get(id),fingerprint,'idempotency key reused across storage requests')
              idempotency.set(id,fingerprint)
              if(p.headers['if-match']) assert.equal(p.headers['if-match'],`"pkv_${rev.get(key)}"`)
              store.set(key,decode(p.body_base64).value);rev.set(key,(rev.get(key)||0)+1);result=response(200,{})
            }
          } else if(url.pathname==='/get_login_info') result=response(200,{status:'ok',retcode:0,data:{user_id:12345678,nickname:'QQ 测试机器人'}})
          else if(url.pathname==='/send_private_msg') {
            assert.equal(p.credential_ref,'cred_mock')
            assert.equal(p.headers.authorization,undefined)
            const payload=decode(p.body_base64);assert.equal(payload.user_id,123456789);assert.equal(payload.message[0].type,'text')
            result=response(200,{status:'ok',retcode:0,data:{message_id:123}})
          } else throw new Error(`unexpected host route ${p.path}`)
        }
        reply=Buffer.from(JSON.stringify({result}));return reply.length
      },
      host_read(ptr,capacity) {
        assert.equal(capacity,reply.length)
        new Uint8Array(instance.exports.memory.buffer,ptr,capacity).set(reply)
        return reply.length
      },
    },
  }
  const module=await WebAssembly.compile(readFileSync(new URL('../build/runtime/plugin.wasm',import.meta.url)))
  instance=await WebAssembly.instantiate(module,imports)
  wasi.initialize(instance)
  const invoke = message => {
    const bytes=Buffer.from(JSON.stringify(message)),ptr=instance.exports.dian115_alloc(bytes.length)
    new Uint8Array(instance.exports.memory.buffer,ptr,bytes.length).set(bytes)
    const packed=instance.exports.dian115_handle(ptr,bytes.length)
    const resultPtr=Number(packed>>32n),resultLength=Number(packed&0xffffffffn)
    return JSON.parse(Buffer.from(instance.exports.memory.buffer,resultPtr,resultLength).toString('utf8'))
  }
  return {invoke,calls,store,module}
}
function runtimeInvoke(op,id,payload={}) { return {method:'runtime.invoke',params:{envelope:{op,invocation_id:id,payload}}} }
function assertBusinessSafe(value) {
  assert.ok(Buffer.byteLength(JSON.stringify(value))<=256*1024,'business response exceeds budget')
  const visit = v => {
    if(Array.isArray(v)) return v.forEach(visit)
    if(v&&typeof v==='object') for(const [k,x] of Object.entries(v)) { assert.ok(!/cookie|_token$|_secret$/.test(k));assert.ok(!['cid','file_id','database_id','absolute_path','raw_path','password','client_secret','webhook_secret','authorization'].includes(k));visit(x) }
    if(typeof v==='string') assert.ok(!/^(?:\/|\\\\|[A-Za-z]:[\\/])/.test(v),`absolute path in response: ${v}`)
  }
  visit(value)
}
test('compiled WASM executes initialize, state/ETag, actions and shutdown with real ABI',async()=>{
  const h=await harness()
  const exports=WebAssembly.Module.exports(h.module).map(x=>x.name)
  for(const name of ['memory','dian115_alloc','dian115_handle']) assert.ok(exports.includes(name))
  assert.deepEqual(h.invoke({method:'runtime.initialize',params:{protocol:'dian115:wasm@1'}}).result,{ready:true,protocol:'dian115:wasm@1'})
  const initial=h.invoke(runtimeInvoke('state','state-1',{view:'main'})).result
  assert.equal(initial.state.config.enabled,false);assertBusinessSafe(initial)
  assert.deepEqual(h.invoke(runtimeInvoke('state','state-2',{view:'main',if_none_match:initial.etag})).result,{etag:initial.etag,not_modified:true})
  const config={...initial.state.config,target_id:'123456789',credential_ref:'cred_mock',adapters:[]}
  assert.equal(h.invoke(runtimeInvoke('action','save-1',{id:'save-config',input:config})).result.status,'succeeded')
  assert.equal(h.invoke(runtimeInvoke('action','connect-1',{id:'test-connection',input:{}})).result.bot.user_id,'12345678')
  assert.equal(h.invoke(runtimeInvoke('action','test-once',{id:'send-test',input:{}})).result.status,'succeeded')
  assert.equal(h.invoke(runtimeInvoke('action','test-once',{id:'send-test',input:{}})).result.status,'succeeded')
  assert.equal(h.calls.filter(x=>x.params?.path.endsWith('/send_private_msg')).length,1)
  assert.equal(h.invoke(runtimeInvoke('action','test-independent',{id:'send-test',input:{}})).result.status,'succeeded')
  assert.equal(h.calls.filter(x=>x.params?.path.endsWith('/send_private_msg')).length,2)
  assertBusinessSafe(h.invoke(runtimeInvoke('state','state-3',{view:'main'})).result)
  assert.deepEqual(h.invoke({method:'runtime.shutdown',params:{}}).result,{stopping:true})
})
test('compiled WASM rejects invalid config and unsupported operations',async()=>{
  const h=await harness()
  assert.ok(h.invoke(runtimeInvoke('nonsense','bad')).error)
  const result=h.invoke(runtimeInvoke('action','bad-cfg',{id:'save-config',input:{url:'http://user:secret@host',target_id:'1'}})).result
  assert.equal(result.status,'failed');assertBusinessSafe(result)
})
test('adapter state respects runtime path boundary while keeping user mappings recoverable',async()=>{
  const h=await harness()
  const config={...h.invoke(runtimeInvoke('state','s')).result.state.config,target_id:'123456789',adapters:[{key:'organize',path:'/api/organize/history',list_pointer:'/data/items',id_pointer:'/id',status_pointer:'/status',title_pointer:'/title',body_pointer:'/detail',levels:{done:'success'}}]}
  assert.equal(h.invoke(runtimeInvoke('action','save',{id:'save-config',input:config})).result.status,'succeeded')
  const result=h.invoke(runtimeInvoke('state','s2')).result
  assertBusinessSafe(result);assert.equal(result.state.config.adapters[0].path,'api/organize/history')
})
