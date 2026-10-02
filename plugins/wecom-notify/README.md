# DIAN115 企业微信通知 0.1.0

使用企业微信自建应用向单成员发送 Emby 新电影／新剧集入库通知。兼容范围 `>=4.0.64 <4.0.65`。

先阅读[配置指南](../../docs/configuration.md)。有公开 TMDB 图片时发送图文，否则使用文字。首次检测建立基线，默认关闭自动通知。Secret 使用宿主托管凭据；Access Token 仅在运行时缓存。

本版图文卡片的跳转 URL 固定为开发环境的 NAS 局域网地址，尚无修改该地址的配置项。只支持单成员，不支持 `@all`，不包含交互机器人。

## 构建

验证环境：Go 1.27.1、Node.js 24。

```sh
npm ci
go test ./runtime
npm run build
npm run check
npm run package -- --generate-key
python scripts/verify-package.py releases/xiaobai.wecom-notify-0.1.0.d115p
```

最后一步需要 Python cryptography。自行生成签名会改变发布者身份，可能影响宿主更新信任。签名私钥不要提交到仓库。

原生 Emby 检测、队列、宿主 ABI 与构包工具沿用 QQ 插件，来源说明见 `NOTICE.md`。
