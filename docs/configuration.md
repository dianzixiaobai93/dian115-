# DIAN115 QQ 与企业微信通知插件配置指南

适用版本：DIAN115 **4.0.64**。当前两个安装包的版本约束均为 `>=4.0.64 <4.0.65`，未验证其他宿主版本。

| 插件 | 安装包 | 消息通道 |
| --- | --- | --- |
| QQ 通知 0.1.2 | `xiaobai.qq-notify-0.1.2.d115p` | NapCat / OneBot 11 HTTP |
| 企业微信通知 0.1.0 | `xiaobai.wecom-notify-0.1.0.d115p` | 企业微信自建应用 |

两者支持 Emby 新电影、新剧集入库通知；有可用的公开 TMDB 图片时发送图文，缺失时使用文字。本文使用占位值，每位使用者应填写自己的配置。

## 一、导入插件

1. 下载对应 `.d115p` 文件，在 DIAN115 插件中心选择本地导入。
2. 核对插件名称、版本、权限及开发者签名，再启用插件。
3. 打开插件配置页，先完成通道测试，再开启自动通知。

安装包采用本地开发者签名。宿主若提示发布者信任，请按实际安装提示处理；不要跳过完整性或签名校验。两个插件可独立使用或同时启用。发布文件附 SHA-256 校验值。

## 二、QQ 通知

### 1. 配置 NapCat

已有 NapCat 时直接复用。在 NapCat WebUI 登录机器人 QQ，创建并启用 **OneBot HTTP Server**：

| 配置项 | 示例 |
| --- | --- |
| Host | `0.0.0.0` |
| Port | `3000` |
| 消息格式 | `Array` |
| Token | 自己设置的 HTTP API Token |

DIAN115 使用 Docker 映射后的地址。例如端口映射 `13000:3000`，插件地址填 `http://NAS_IP:13000`；映射 `3000:3000` 时填 `http://NAS_IP:3000`。共享 Docker 网络时也可使用容器名称。不同容器之间通常不能用 `localhost` 指向对方。

HTTP API Token 与 NapCat WebUI Token 不同，插件需要前者。NapCat 官方提供 [WebUI 配置指南](https://doc.napneko.icu/config/basic) 和 [OneBot HTTP API 文档](https://napneko.github.io/api/)。

### 2. 配置 DIAN115

1. 填 NapCat HTTP API 地址，只填服务地址，不追加发送接口路径。
2. 选择私聊或群聊，填接收 QQ 号或群号。私聊目标先添加机器人为好友；群聊先将机器人加入目标群。
3. 输入 HTTP Token，使用插件的安全保存功能交给 DIAN115 托管，再保存配置。
4. 依次执行“测试连接”和“发送测试消息”，在 QQ 客户端核对。

若前端凭据绑定受宿主管理认证限制，可在插件管理的托管凭据中创建静态 Header 绑定，再填写生成的 `credential_ref`：

| 字段 | 值 |
| --- | --- |
| host | NapCat 地址中的主机名或 IP，不能带端口、协议 |
| method | `POST` |
| path_prefix | `/` |
| injection_mode | `static` |
| location | `header` |
| name | `authorization` |
| prefix | `Bearer `，末尾保留一个空格 |
| secret | NapCat HTTP API Token |

插件通过宿主注入 `Authorization: Bearer <HTTP_TOKEN>`，配置中保存凭据引用，不需要自定义 Header 输入框。不要把已经带 `Bearer ` 前缀的完整 Header 再当成 Token 输入。

### 3. 排查

| 现象 | 优先检查 |
| --- | --- |
| HTTP 401／403 | HTTP Token 是否正确；host、POST、Header 绑定是否匹配；保存后 credential_ref 是否存在 |
| 连接失败／HTTP 502 | QQ 是否仍登录；HTTP Server、Docker 端口、容器网络是否可达 |
| 测试写入 HTTP 409 | 核对插件是否为 0.1.2；此版修复存储路径之间的幂等键冲突，仍失败时保留脱敏诊断信息 |
| 文字成功，图片缺失 | TMDB 图片权限、公开图片地址、NapCat 图片获取是否正常 |

## 三、企业微信通知

本插件使用 **企业微信自建应用**，不是群机器人 Webhook，也不是个人微信直接推送。

### 1. 准备应用

在企业微信管理后台获取企业 ID（CorpID）、自建应用 ID（AgentID）、应用 Secret 和接收成员的实际 **UserID**。成员显示名不能代替 UserID，并确保该成员在应用可见范围内。官方 [发送应用消息文档](https://developer.work.weixin.qq.com/document/path/90236) 说明了接收范围及无效成员返回值。

在应用的企业可信 IP 中配置实际调用企业微信 API 的出口公网 IP；插件通过宿主直接访问 API 时，通常是 NAS 的出口 IP。若已有网关或转发，则按实际出口配置，不能把局域网 IP 当作公网出口 IP。

### 2. 配置 DIAN115

1. API 地址保持 `https://qyapi.weixin.qq.com`。
2. 填 CorpID、AgentID 和接收成员 UserID。
3. 输入应用 Secret，点击“绑定 Secret”，再保存插件配置。
4. 执行连接／发送测试，在企业微信应用会话中核对消息。

若需要在插件管理端手动绑定凭据，使用：

| 字段 | 值 |
| --- | --- |
| host | `qyapi.weixin.qq.com` |
| method | `GET` |
| path_prefix | `/cgi-bin/gettoken` |
| injection_mode | `static` |
| location | `query` |
| name | `corpsecret` |
| prefix／suffix | 留空 |
| secret | 自建应用 Secret |

将生成的 `credential_ref` 填入插件。Secret 由宿主注入获取 Token 的请求，Access Token 只在插件运行时缓存。更换 Secret 后重新绑定凭据并保存。

当前版本只支持单成员，不支持 `@all`。**0.1.0 图文卡片点击地址固定为开发时的 NAS 局域网地址，其他部署下点击跳转可能不可用；通知发送不依赖该跳转地址。** 此版尚无配置该跳转地址的选项。

单向通知不需要配置“API 接收消息”的 URL、Token、EncodingAESKey。交互机器人是另一套独立服务，不包含在此通知安装包中。

### 3. 排查

| 现象 | 优先检查 |
| --- | --- |
| 获取应用凭证失败 | CorpID、Secret、凭据引用及 query 注入配置 |
| 可信 IP 错误 | 当前实际出口公网 IP 是否在该应用的可信 IP 列表 |
| 无效接收成员 | 使用实际 UserID，核对应用可见范围 |
| 图文失败或变成文字 | TMDB 图片是否存在，图片权限和企业微信返回错误 |

## 四、启用 Emby 入库通知

两种插件分别配置，可使用相同的 Emby 来源：

1. 确认 DIAN115 已连接 Emby，插件已获得宿主 Emby 读取权限。
2. 在通知插件中选择 Emby 实例并保存，再读取和选择媒体库。
3. 启用新电影（Movie）和新剧集（Episode）检测；建议先使用默认 60 秒轮询间隔。
4. 开启自动通知并保存，首次检测只建立基线，不补发已有影片。
5. 之后新增一个电影或剧集，查看通知记录及接收端。测试预览不等于真实新增媒体检测。

修改 Emby 实例或媒体库会重新建立基线。检测以 Emby 入库时间为准；保留旧入库时间的重新导入、移动已有条目或修改元数据不保证触发新通知。标题、评分、季集等依赖实际元数据；缺失的画质、大小、耗时不会编造。

失败时先检查通道、凭据和队列诊断，再按插件界面重试。重试可能产生重复消息，不能仅凭发送接口返回成功认定最终送达。

## 五、源码与重新构建

`plugins/qq-notify` 与 `plugins/wecom-notify` 是独立项目。安装包无需本地编译。构建验证使用 Go 1.27.1、Node.js 24；参考命令在各插件 README 中。

签名私钥、用户配置、Token、Secret、浏览器数据、SSH 密钥不应提交到仓库。自行构包使用自己的 Ed25519 签名；更换签名身份可能影响宿主对更新发布者的信任。

插件契约与构包工具的来源说明保留在 `NOTICE.md`。公开插件资料参考 [DIAN115 插件平台文档](https://github.com/madbrolab/dian115/tree/main/docs/plugin-platform)。当前发布用于本地导入，源码中的市场下载 URL 为占位值，不能当成正式插件市场源。
