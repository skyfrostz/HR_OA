# 商业审查首批后端补强

2026-11-07，基线 `84b9809cc226fca820714b47c373f1f54a3a0a56`。未提交或推送。本轮只修改 `server/oa/`。

- 新增 `POST /api/oa/v1/employees/search`：请求体文字查询、精确部门筛选与分页，旧 GET 列表保留。
- 新增 `POST /api/oa/v1/employees/export.xlsx`：复用查询筛选及行/字段权限，导出所有匹配行；5000 上限，零结果只有表头。旧 GET 导出保留。
- 字面查询使用绑定参数的 MySQL LOCATE，百分号、下划线、反斜杠和引号不变成模式/SQL。OR 显式分组，始终与既有员工本人范围 AND。
- query 去首尾空白后最多100 Unicode字符；部门ID可空、非空必须正数；分页省略默认1/20，非法/溢出拒绝。HTTP JSON 严格检查对象、未知字段、大小，查询内容不进入 URL。
- 不新增索引或表迁移。非空子串查询可能扫描授权范围；本次没有负载测试，不声称支持大规模人员库。计数与页数据沿既有两次读，未承诺并发修改下的快照一致性。

验证：

- `go test -mod=readonly -count=1 ./oa/...` PASS。
- `go test -mod=readonly -race -count=1 ./oa/...` PASS。
- `go build -mod=readonly -o /tmp/hr-oa-commercial-api ./cmd/oa` PASS。
- 使用 pinned swag parser 解析33个路由，新POST搜索/导出均存在，PASS。
- `git diff --check -- server/oa` PASS。
- 新增测试覆盖分页/Unicode/非法查询、NULL与未知字段、SQL参数与OR分组、本人范围、未绑定拒绝、查阅员投影及路由权限。
- 真实MySQL和组合浏览器验收由QA在本轮新报告记录，不能用上述单元PASS代替。

未在本轮修复：反代共享登录限流、账号生命周期、合同状态流转、工资与年假变更历史、生产审批规则、敏感预览清理、可观测性与真实集成。它们已进入综合审查清单，不应被搜索功能补强掩盖。

## QA 实测补充及职责移交

用户于本轮明确要求整体统筹交回总控，本会话已发送完整上下文并恢复仅后端职责。总控已接回任务；其负责综合报告、前端视觉与最终交付结论。

已读取QA实际日志（不是重跑或推测）：

- `.local/commercial-api-1.log`：真实MySQL字面搜索、严格输入、分页、四角色隔离与筛选XLSX **PASS**。覆盖姓名/工号/部门、百分号/下划线/引号、SQL样式输入、精确本部门不含子部门、工资投影与空导出。
- `.local/commercial-limit-1.log`：隔离临时MySQL中，管理员及查阅员恰好5000条完整导出、5001条拒绝 **PASS**；临时容器和网络清理完成，现有库没有注入这批5001记录。
- `.local/commercial-restart-1.log`：API自动恢复后健康，原模拟档案及附件字节读回 **PASS**。Web恢复由QA/总控汇总。

未发现需修复的本轮后端失败。`COMMERCIAL_BACKEND_SHA256SUMS` 绑定当前所有OA Go源码；保持冻结供总控/QA验收，不替代历史清单。不将本轮搜索补强等同生产可用，反代限流和其它未实现项继续由总控排期。
