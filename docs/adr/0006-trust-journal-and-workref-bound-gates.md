---
status: accepted
---

# Trust Journal 与 WorkRef-bound Gate

Evidence、Execution、Review 和 GateReceipt 进入 append-only Trust Journal，但 Trust Journal 不拥有 Workflow Task status。每个 record 通过 `WorkRef` 引用 Lite action 或 GSD entity，并绑定 workflow/source digest、Git tree digest 和 evidence-set digest。

任一绑定改变后尚未消费的 Review/Gate 自动 stale。Terminal transition 必须先 staging 出 exact successor；GateReceipt 只记录 `verified|limited|failed` evaluation，并绑定 SkillPlan、required Gate set 与实际 Policy digest；这些摘要必须从 Engine-written Contract Registry 反查 exact canonical bytes。`failed` 永不授权；`limited` 需要 Policy 允许和模型/Coordinator 无法伪造的 trusted Host user-interaction Receipt，ActorRef 只做 provenance；只有可授权结果才能生成 CompletionAuthorization。Authorization 绑定 previous/successor digest、fencing epoch、tree/evidence-set 和相同 Plan/Gate/Policy/acceptance refs，完全匹配的 CAS checkpoint 才能消费。Cancellation 使用独立 Authorization，不构成 Completion Claim。Evidence 同时表达 capture trust 与 proof scope；低等级或范围不足的证据不能支持高等级 Claim。

这个选择吸收 Harness Anything 的 immutable/gate 思想与 Missions 的 Claim Coverage，但不复制它们的完整对象模型或 CSV authority。
