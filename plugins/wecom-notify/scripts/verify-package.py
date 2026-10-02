#!/usr/bin/env python3
"""Independent ZIP/hash/Ed25519 verification. Requires Python cryptography."""
import base64,hashlib,json,pathlib,sys,zipfile
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

def decode(v): return base64.urlsafe_b64decode(v+'='*((-len(v))%4))
def canonical(v): return json.dumps(v,ensure_ascii=False,separators=(',',':'),sort_keys=True).encode('utf-8')
path=pathlib.Path(sys.argv[1])
assert path.stat().st_size <=32*1024*1024
with zipfile.ZipFile(path) as archive:
 names=archive.namelist()
 assert len(names)==len(set(names)) and len(names)<=1024
 assert all(not n.startswith('/') and not any(p in ('','..','.') for p in n.split('/')) and '\\' not in n and ':' not in n for n in names)
 assert not any(n.endswith('.pem') or 'private' in n.lower() for n in names)
 assert sum(i.file_size for i in archive.infolist())<=128*1024*1024
 for i in archive.infolist(): assert i.file_size<=32*1024*1024 and (i.external_attr>>16)&0o777 == 0o644
 manifest=json.loads(archive.read('manifest.json'));integrity=json.loads(archive.read('integrity.json'));signature=json.loads(archive.read('signature.json'))
 assert manifest['runtime']['kind']=='wasm' and manifest['runtime']['protocol']=='dian115:wasm@1'
 assert manifest['ui']['mode']=='federation' and manifest['ui']['federation']['entry'] in names
 assert archive.read(manifest['runtime']['entry'])[:8]==b'\0asm\x01\0\0\0'
 declared=[f['path'] for f in integrity['files']]
 assert declared==sorted(declared,key=lambda x:x.encode('utf-8'))
 assert set(declared)==set(names)-{'integrity.json','signature.json'}
 for f in integrity['files']:
  data=archive.read(f['path']);assert len(data)==f['size'];assert hashlib.sha256(data).hexdigest()==f['sha256']
 public=decode(signature['public_key'])
 key_id='ed25519:'+base64.urlsafe_b64encode(hashlib.sha256(public).digest()).decode().rstrip('=')
 assert key_id==manifest['publisher']['key_id']==signature['key_id']
 assert signature['algorithm']=='Ed25519' and signature['canonicalization']=='RFC8785-JCS' and signature['domain']=='DIAN115-PLUGIN-PACKAGE-V1'
 message=b'DIAN115-PLUGIN-PACKAGE-V1\0'+canonical(manifest)+b'\0'+canonical(integrity)
 Ed25519PublicKey.from_public_bytes(public).verify(decode(signature['signature']),message)
 print('PASS: ZIP members, safe paths, file modes, WASM magic, integrity hashes and Ed25519 signature')
 print('Package SHA-256: '+hashlib.sha256(path.read_bytes()).hexdigest())
