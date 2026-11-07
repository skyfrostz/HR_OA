# M6 外部接口与源码来源

核对日期：2026-11-07。在既有 Go、Pure Admin 和 Vant 架构内完成集成，不引入第二套身份或工作流引擎。

| 来源 | 用途 | 核对情况 |
| --- | --- | --- |
| [企业微信网页授权](https://developer.work.weixin.qq.com/document/path/91022) | snsapi_base、code、state、回调协议 | 官方页面经 agent-reach Jina 网页读取成功；直接浏览器抓取失败。state 使用字母数字且不超过128字节，code 上限512字节；用户标识必须显式映射 OA 账号。 |
| [企业微信获取访问用户身份](https://developer.work.weixin.qq.com/document/path/91023) | 服务端换取身份 | 官方2026-04-22页面经 Jina 已读；当前为 auth/getuserinfo，返回 userid；不把仅返回 openid 的外部成员作为员工登录。 |
| [企业微信网页登录](https://developer.work.weixin.qq.com/document/path/98152)、[网页登录身份](https://developer.work.weixin.qq.com/document/path/98176) | 常规 PC 浏览器扫码/桌面授权 | 两份官方正文经 Jina 已读；使用 login.work.weixin.qq.com 的 CorpApp 重定向，换身份仍用 auth/getuserinfo；无需在 OA 嵌入第三方脚本。 |
| [企业微信应用消息](https://developer.work.weixin.qq.com/document/path/90236) | 应用消息投递 | 官方正文已读；成功返回仍需检查 invaliduser/unlicenseduser，重复检查有时限。只发送通用提示，API 接受不代表送达或已读。 |
| [企业微信 access_token](https://developer.work.weixin.qq.com/document/path/91039) | 服务端令牌缓存与刷新 | 官方正文已读；令牌可提前失效，必须在服务端缓存并受控刷新，不能下发给浏览器。 |
| [阿里云 OSS Go SDK V2 官方指南](https://www.alibabacloud.com/help/en/oss/developer-reference/manual-for-go-sdk-v2/) | 私有对象读写、签名、CRC和超时 | 官方2026-10-14版本页面已读。采用官方 alibabacloud-oss-go-sdk-v2 v1.6.1，依赖锁定在 go.mod/go.sum。 |

官方 SDK 指南包含全权限示例，本项目不照搬该权限范围；部署文档应给出限定 Bucket/前缀的所需权限。端点、自定义域和区域需按客户账号当时要求配置，不能从模拟服务验收推导真实云资源可用。

当前 GVA BSL 1.1 和 Pure MIT 来源沿用 UPSTREAM.md/PURE_ADMIN_UPSTREAM.json。OSS SDK v1.6.1 的模块缓存 LICENSE 已核对为 Apache-2.0，[原始许可文本](upstream/oss-go-sdk-v2-LICENSE)随源码保存；通过 Go 模块依赖复用，没有修改或复制签名实现。模拟服务和本地回归是测试替身，不是第三方官方认证。原始网页获取临时文件不进入项目分发。

PDF 渲染复用锁定的 vue-pdf-embed 2.1.4（MIT）/ pdfjs-dist 4.10.38（Apache-2.0）。仅使用字节到 canvas 的分页渲染，不启用脚本、annotation 链接、表单或文字层，禁止 eval/XFA 和 worker 外部资源请求。2026-11-07 核对 Mozilla 官方 [GHSA-wgrm-67xf-hhpq](https://github.com/mozilla/pdf.js/security/advisories/GHSA-wgrm-67xf-hhpq) 和 [GHSA-hq66-cqwq-w95j](https://github.com/mozilla/pdf.js/security/advisories/GHSA-hq66-cqwq-w95j)，4.10.38 不在这两条公告列出的受影响版本范围内；这不构成完整依赖漏洞审计。

前端沿用 Pure/Vant 组件和现有 OA 会话、远程请求与版本写入机制；本轮没有整体并入另一套前后端框架。本地打包 CMaps、标准字体及全部 LICENSE notices，未新增前端依赖或 CDN。

应用现有 frontend-design、redesign-existing-projects、vue-best-practices 与 playwright skills，未安装新的技能或修改全局包配置。沿用现有 Pure 主题和 Vant 移动交互，集成状态和导入成功提示按真实截图修正对比度；技能摘要及过程记录保存在本机协调目录。
