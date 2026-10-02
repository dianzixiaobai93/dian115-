# DIAN115 QQ 与企业微信通知插件

用于 Emby 新电影、新剧集入库通知。目前两个插件均限定 DIAN115 4.0.64。

| 插件 | 版本 | 安装文件 |
| --- | --- | --- |
| QQ / NapCat | 0.1.2 | [下载](releases/xiaobai.qq-notify-0.1.2.d115p) |
| 企业微信自建应用 | 0.1.0 | [下载](releases/xiaobai.wecom-notify-0.1.0.d115p) |

导入对应 `.d115p`，配置通知通道并测试，再启用 Emby 自动检测。首次检测建立基线，不补发旧库。

**完整步骤：[配置指南](docs/configuration.md)**，包含 Token／Secret 托管绑定、403／409 排查、企业可信 IP、UserID、Emby 来源设置和版本限制。

QQ 支持私聊／群聊，兼容 OneBot 11 HTTP Array 消息。企业微信使用自建应用向单成员通知；不是群机器人 Webhook。企业微信 0.1.0 的图文跳转地址固定为开发环境地址，其他部署下跳转可能不可用。

`plugins/qq-notify` 与 `plugins/wecom-notify` 为独立源码项目；`releases/` 为签名安装包；[SHA256SUMS](releases/SHA256SUMS) 用于校验。

本仓库只发布通知插件，不包含企业微信交互机器人、NAS 配置、账号凭据、签名私钥或 DIAN115 主项目私有源码。来源说明见各插件 `NOTICE.md`。
