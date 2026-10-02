# DIAN115 QQ 通知 · 0.1.2

## 0.1.2 修复

- 持久化幂等键加入存储路径，修复独立测试发送之间的 HTTP 409 冲突。
- Emby 电影／剧集通知发送 OneBot 11 Array 消息：公开 TMDB 背景图（缺失时海报）在前，文字在后。
- 显示已获取的标题、年份、季集、评分、类型、分类、地区和 Emby 编号；缺失字段省略，不伪造大小、画质或耗时。
- 剧集从匹配的 Emby Series 获取 TMDB ID，避免把单集 ID 当成电视剧 ID。
- 401/403 提示核对 HTTP Token 和托管凭据；502 提示检查 QQ 登录、端口和网络。不回显 Token。
- Emby 开启时，“发送测试消息”使用最新电影／剧集，标记为测试预览，不推进入库游标。
- 新增可选 TMDB 元数据权限。继续使用宿主 credential_ref 注入 Authorization: Bearer，不增加自定义 Header 输入框。

此版本使用新生成的本地开发签名，签名私钥不包含在源码压缩包中。重新打包需要自有 Ed25519 私钥，或 `npm run package -- --generate-key` 生成新的开发签名。


本地开发的 DIAN115 第三方插件，通过宿主 Broker 调用 NapCat / OneBot 11 HTTP 服务。

此修订修正官方示例遗留的 3.x 版本门槛，安装目标限定为用户当前 DIAN115 4.0.64。实际运行兼容性仍以 NAS 验收结果为准；未将范围泛化到所有 4.x。

## 已实现

- QQ 群和私聊；连接测试、测试消息。
- Access Token 交给 DIAN115 托管；插件只保存 `credential_ref`。前端创建绑定若遇到宿主管理端认证限制，可在插件管理中创建再填写引用。
- 默认关闭自动通知；首次启用每个新来源时建立基线，跳过已有记录。
- 明确选择宿主 Emby 实例和媒体库；电影与剧集新入库通知；最多 50 条一页，分页读取增量。
- 消息前缀、模板、通知级别筛选。
- 持久化游标、outbox、发送结果及最近 64 条日志。
- 网络错误、HTTP 429/5xx 按 10、20、40…秒间隔重试；总尝试次数可配置为 1–5。401/403 等错误、OneBot 业务失败及无有效收据响应留待人工处理。
- 手动检测；用当前配置重试失败队列（已确认失败的消息使用当前目标及凭据，仍待确认的消息保留原目标）。
- 历史/后台任务接口诊断与用户显式配置的 JSON Pointer 适配器。

## 当前边界

这个版本不是把所有 Telegram 系统通知复制到 QQ。公开插件契约未定义通用系统通知广播。

整理、STRM、115 转存、任务异常等通知需要按实际宿主响应配置适配器，默认为空。官方 OpenAPI 的这几个接口使用 `GenericHostObject`，没有固定字段契约。诊断样本后才能确认具体状态、列表和分页字段；插件不猜字段、不把匹配失败当成空列表。

Emby 检测以 `date_created` 为基准。重新导入保留旧时间的媒体、修改既有条目、将旧条目移动到所选媒体库不会保证触发新入库通知。改变实例或媒体库会重新建立基线；启用前已存在的条目不通知。一次扫描最多 1000 条、队列最多 200 条，超过边界会报告错误而不推进对应游标，需要先处理队列或缩小范围。界面筛选在入队时生效，修改筛选不会撤回已经排队的消息。

OneBot 发送接口没有可用的幂等键。本插件正常运行会去重，且测试调用按 invocation ID 保存收据；但网络超时或「QQ 已接收而发送结果尚未落盘」期间崩溃可能造成重复。它提供持久化的至少一次尝试语义，不能保证恰好一次。日志中的成功表示 NapCat 已返回有效成功收据，最终送达请在 QQ 核对。

## 安装与实机接入

1. 在 DIAN115 插件中心选择本地导入 `xiaobai.qq-notify-0.1.2.d115p`。本包按官方示例签名，发布者为「小白 · 本地开发」，未上传市场。
2. 宿主需兼容 `>=4.0.64 <4.0.65`、Plugin API v2，并支持 WASM resident。具体能力以安装权限检查为准；Emby/历史接口声明为可选，旧宿主不提供时相应操作会报错。
3. 填写现有 NapCat HTTP API origin，例如 `http://NAS_IP:13000`，填写目标群号/QQ 号，保存托管凭据引用。
4. 保存配置，测试连接，再发送测试消息。测试使用已保存的配置。
5. 明确选择 Emby 实例并保存，再读取媒体库；选择范围后开启检测和自动通知，再保存。
6. 第一次检测建立基线，后续新增媒体才发送。后台约每 10 秒检查命令和重试队列，实际扫描间隔默认 60 秒。

已有 NapCat 无需再次部署。HTTP API 的端口以现有 Docker 端口映射为准，WebUI Token 不是 OneBot Access Token。`http://napcat:3000` 仅适用于共享 Docker network 的容器，通常填写 NAS 局域网 IP 和已映射端口更方便。完整配置步骤见[配置指南](../../docs/configuration.md)。

托管凭据静态 Header 绑定：hostname 为 NapCat 主机名/IP，method `POST`，path_prefix `/`，location `header`，name `authorization`，prefix `Bearer `（末尾有空格），secret 为 HTTP API Access Token。Token 不写入插件配置或浏览器存储。内网 HTTP 会明文传输凭据。

## 历史适配器

先在插件「高级」中读取脱敏样本，确认实际返回结构。以下仅解释格式，字段为示意，不是 DIAN115 已验证字段：

```json
[
  {
    "key": "organize",
    "path": "/api/organize/history?实际分页参数=实际值",
    "list_pointer": "/实际列表路径",
    "id_pointer": "/实际记录ID字段",
    "status_pointer": "/实际状态字段",
    "title_pointer": "/实际标题字段",
    "body_pointer": "/实际正文或错误字段",
    "levels": {"实际成功终态": "success", "实际失败终态": "failed"}
  }
]
```

只允许 `/api/organize/history`、`/api/tasks`、`/api/history` 三个路径，可带经过真实响应验证的查询参数。JSON Pointer 相对列表项定位字段，正文可留空。空的 list_pointer 表示响应自身就是列表。levels 未列出的状态不发送；同一个记录从进行中变成终态可触发一次。

只检查该查询返回的当前窗口，不自动猜分页参数、不保证长时间离线或窗口外事件完整；窗口最多 1000 条，最近 2000 个记录/状态指纹用于去重。接口若返回 200 中的业务错误会因列表结构不匹配而报告失败。诊断最多展示 5 条样本，隐藏敏感字段、路径和带链接的文本。

## 本地重新构建

需要 Node.js 20+ 和 Go 1.25+（官方示例实际 go.mod 需要 1.25；README 的旧最低版本描述不足以编译 reactor 导出）。

```sh
npm ci
npm run build
npm run check
npm test
npm run package -- --generate-key
```

`npm test` 包含 Go 业务测试与 Node WASI 实际编译模块的 ABI 测试。`npm run package` 使用长期同一 Ed25519 签名密钥；生成新密钥会改变发布者身份，可能影响宿主更新信任。私钥不在源码归档和安装包中，单独的签名备份仅用于后续版本构建。若没有原密钥，可生成新身份的本地开发包，不能称为同一发布者更新。

`releases/*.d115p` 是安装文件；`build/` 是中间产物。市场条目 URL 为 `example.invalid` 占位地址，因为本项目仅做本地导入，不能把它作为真实市场下载源。

## 来源与验证边界

依据用户提供的 `dian115-main.zip`，提交 `7637a392e9512bd0f7965b99445768969eb075fc`，归档时间 2026-10-01。使用其 `docs/plugin-platform/` 契约、完整插件示例的 ABI、构建和签名脚本以及黑盒检查器，不依赖 DIAN115 主项目私有源码。

本地已验证业务流程、实际 WASM 的 initialize/state/ETag/action/shutdown、官方项目契约检查及安装包完整性和 Ed25519 签名。开发环境已完成实机导入、托管凭据及测试消息验证；其他部署仍需按配置指南验收。

官方参考：
- https://github.com/madbrolab/dian115/tree/main/docs/plugin-platform
- https://github.com/NapNeko/NapCat-Docker
- https://github.com/botuniverse/onebot-11
