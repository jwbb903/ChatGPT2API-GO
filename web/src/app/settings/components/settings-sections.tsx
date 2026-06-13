"use client";

import { LoaderCircle, PlugZap } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { testProxy, type ProxyTestResult } from "@/lib/api";

import { useSettingsStore } from "../store";

/**
 * 6 个 section 组件集中放这里：
 *   - 都只负责呈现自己 section 的字段，store 逻辑共用
 *   - 不再带保存按钮——保存统一走 FloatingSaveBar
 *   - 视觉只用 grid + 输入框，不再每个 section 套 Card（避免 Card 套 Card 套 Card）
 *
 * 共享样式常量挑出来，避免几十处 className 重复。
 */
const INPUT_CLASS = "h-10 rounded-xl border-stone-200 bg-white";
const LABEL_CLASS = "text-sm text-stone-700";
const HELP_CLASS = "text-xs text-stone-500";
const TILE_CLASS = "rounded-xl border border-stone-200 bg-white px-4 py-3";

/* ───────────────────────── 账号 ───────────────────────── */

export function AccountSection() {
  const config = useSettingsStore((s) => s.config);
  const setRefreshAccountIntervalMinute = useSettingsStore((s) => s.setRefreshAccountIntervalMinute);
  const setAutoRemoveInvalidAccounts = useSettingsStore((s) => s.setAutoRemoveInvalidAccounts);
  const setAutoRemoveRateLimitedAccounts = useSettingsStore((s) => s.setAutoRemoveRateLimitedAccounts);

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <div className="space-y-2 md:col-span-2">
        <label className={LABEL_CLASS}>账号刷新间隔（分钟）</label>
        <Input
          value={String(config?.refresh_account_interval_minute || "")}
          onChange={(e) => setRefreshAccountIntervalMinute(e.target.value)}
          placeholder="5"
          className={INPUT_CLASS + " md:max-w-xs"}
        />
        <p className={HELP_CLASS}>控制账号自动刷新频率。</p>
      </div>
      <label className={`flex items-center gap-3 ${TILE_CLASS} text-sm text-stone-700`}>
        <Checkbox
          checked={Boolean(config?.auto_remove_invalid_accounts)}
          onCheckedChange={(c) => setAutoRemoveInvalidAccounts(Boolean(c))}
        />
        自动移除异常账号
      </label>
      <label className={`flex items-center gap-3 ${TILE_CLASS} text-sm text-stone-700`}>
        <Checkbox
          checked={Boolean(config?.auto_remove_rate_limited_accounts)}
          onCheckedChange={(c) => setAutoRemoveRateLimitedAccounts(Boolean(c))}
        />
        自动移除限流账号
      </label>
    </div>
  );
}

/* ───────────────────────── 网络 ───────────────────────── */

export function NetworkSection() {
  const [isTestingProxy, setIsTestingProxy] = useState(false);
  const [proxyTestResult, setProxyTestResult] = useState<ProxyTestResult | null>(null);
  const config = useSettingsStore((s) => s.config);
  const setProxy = useSettingsStore((s) => s.setProxy);

  const handleTestProxy = async () => {
    const candidate = String(config?.proxy || "").trim();
    if (!candidate) {
      toast.error("请先填写代理地址");
      return;
    }
    setIsTestingProxy(true);
    setProxyTestResult(null);
    try {
      const data = await testProxy(candidate);
      setProxyTestResult(data.result);
      if (data.result.ok) {
        toast.success(`代理可用（${data.result.latency_ms} ms，HTTP ${data.result.status}）`);
      } else {
        toast.error(`代理不可用：${data.result.error ?? "未知错误"}`);
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "测试代理失败");
    } finally {
      setIsTestingProxy(false);
    }
  };

  return (
    <div className="space-y-3">
      <label className={LABEL_CLASS}>全局代理</label>
      <Input
        value={String(config?.proxy || "")}
        onChange={(e) => {
          setProxy(e.target.value);
          setProxyTestResult(null);
        }}
        placeholder="http://127.0.0.1:7890"
        className={INPUT_CLASS}
      />
      <p className={HELP_CLASS}>留空表示不使用代理。代理同时影响生图请求和上游 OpenAI 转发。</p>
      {proxyTestResult ? (
        <div
          className={`rounded-xl border px-3 py-2 text-xs leading-6 ${
            proxyTestResult.ok
              ? "border-emerald-200 bg-emerald-50 text-emerald-800"
              : "border-rose-200 bg-rose-50 text-rose-800"
          }`}
        >
          {proxyTestResult.ok
            ? `代理可用：HTTP ${proxyTestResult.status}，用时 ${proxyTestResult.latency_ms} ms`
            : `代理不可用：${proxyTestResult.error ?? "未知错误"}（用时 ${proxyTestResult.latency_ms} ms）`}
        </div>
      ) : null}
      <div className="flex justify-end">
        <Button
          type="button"
          variant="outline"
          className="h-9 rounded-xl border-stone-200 bg-white px-4 text-stone-700"
          onClick={() => void handleTestProxy()}
          disabled={isTestingProxy}
        >
          {isTestingProxy ? <LoaderCircle className="size-4 animate-spin" /> : <PlugZap className="size-4" />}
          测试代理
        </Button>
      </div>
    </div>
  );
}

/* ───────────────────────── 图片 ───────────────────────── */

export function ImageSection() {
  const config = useSettingsStore((s) => s.config);
  const setBaseUrl = useSettingsStore((s) => s.setBaseUrl);
  const setImageRetentionDays = useSettingsStore((s) => s.setImageRetentionDays);
  const setCleanupProtectGallery = useSettingsStore((s) => s.setCleanupProtectGallery);
  const setCleanupProtectUserImages = useSettingsStore((s) => s.setCleanupProtectUserImages);
  const setImagePollTimeoutSecs = useSettingsStore((s) => s.setImagePollTimeoutSecs);
  const setImageAccountConcurrency = useSettingsStore((s) => s.setImageAccountConcurrency);

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <label className={LABEL_CLASS}>图片访问地址</label>
        <Input
          value={String(config?.base_url || "")}
          onChange={(e) => setBaseUrl(e.target.value)}
          placeholder="https://example.com"
          className={INPUT_CLASS}
        />
        <p className={HELP_CLASS}>用作生成结果 URL 的前缀。留空则按请求 host 自动推断。</p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <label className={LABEL_CLASS}>图片轮询超时（秒）</label>
          <Input
            value={String(config?.image_poll_timeout_secs || "")}
            onChange={(e) => setImagePollTimeoutSecs(e.target.value)}
            placeholder="120"
            className={INPUT_CLASS}
          />
          <p className={HELP_CLASS}>等待上游图片结果的最长时间。</p>
        </div>
        <div className="space-y-2">
          <label className={LABEL_CLASS}>单账号图片并发</label>
          <Input
            value={String(config?.image_account_concurrency || "")}
            onChange={(e) => setImageAccountConcurrency(e.target.value)}
            placeholder="3"
            className={INPUT_CLASS}
          />
          <p className={HELP_CLASS}>限制每个账号同时处理的图片请求数量。</p>
        </div>
      </div>

      <div className="space-y-3 rounded-xl border border-stone-200 bg-stone-50/60 p-4">
        <div className="space-y-2">
          <label className={LABEL_CLASS}>本地保留天数</label>
          <Input
            value={String(config?.image_retention_days || "")}
            onChange={(e) => setImageRetentionDays(e.target.value)}
            placeholder="30"
            className={INPUT_CLASS + " md:max-w-xs"}
          />
          <p className={HELP_CLASS}>
            自动删除多少天前的本地图片。下面两项保护开关默认开启，避免清理把"还在用的图"也删掉造成画廊裂图或用户作品凭空消失。
          </p>
        </div>
        <label className={`flex items-start gap-3 ${TILE_CLASS} text-sm text-stone-700`}>
          <Checkbox
            checked={Boolean(config?.cleanup_protect_gallery ?? true)}
            onCheckedChange={(c) => setCleanupProtectGallery(Boolean(c))}
          />
          <div className="space-y-1">
            <div className="font-medium">保护画廊已发布的图片</div>
            <div className="text-xs leading-5 text-stone-500">
              发布到画廊视为用户主动表示"这张图有保留价值"。关闭后画廊瓦片可能因 PNG 被删变成裂图。
            </div>
          </div>
        </label>
        <label className={`flex items-start gap-3 ${TILE_CLASS} text-sm text-stone-700`}>
          <Checkbox
            checked={Boolean(config?.cleanup_protect_user_images ?? true)}
            onCheckedChange={(c) => setCleanupProtectUserImages(Boolean(c))}
          />
          <div className="space-y-1">
            <div className="font-medium">保护用户「我的作品」</div>
            <div className="text-xs leading-5 text-stone-500">
              保留所有有归属密钥的图。匿名 / admin 自己生成的无归属图仍按 mtime 清理。关闭后所有过期图按一刀切删除。
            </div>
          </div>
        </label>
      </div>
    </div>
  );
}

/* ───────────────────────── 内容安全 ───────────────────────── */

export function SecuritySection() {
  const config = useSettingsStore((s) => s.config);
  const setGlobalSystemPrompt = useSettingsStore((s) => s.setGlobalSystemPrompt);
  const setSensitiveWordsText = useSettingsStore((s) => s.setSensitiveWordsText);

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <label className={LABEL_CLASS}>全局附加指令</label>
        <Textarea
          value={String(config?.global_system_prompt || "")}
          onChange={(e) => setGlobalSystemPrompt(e.target.value)}
          placeholder="例如：先判断用户提示词是否合规；遇到违法、色情、暴力、仇恨等请求时拒绝回答。"
          className="min-h-28 rounded-xl border-stone-200 bg-white font-mono text-xs shadow-none"
        />
        <p className={HELP_CLASS}>
          每次请求都会作为 system 消息注入。可用于审核用户提示词、统一约束模型行为或固定角色设定。
        </p>
      </div>
      <div className="space-y-2">
        <label className={LABEL_CLASS}>敏感词</label>
        <Textarea
          value={(config?.sensitive_words || []).join("\n")}
          onChange={(e) => setSensitiveWordsText(e.target.value)}
          placeholder="一行一个，命中即拒绝"
          className="min-h-28 rounded-xl border-stone-200 bg-white font-mono text-xs shadow-none"
        />
        <p className={HELP_CLASS}>用户请求包含任意敏感词时直接返回拒绝，不再下发到生图账号。</p>
      </div>
    </div>
  );
}

/* ───────────────────────── AI 审核 ───────────────────────── */

export function AIReviewSection() {
  const config = useSettingsStore((s) => s.config);
  const setAIReviewField = useSettingsStore((s) => s.setAIReviewField);

  return (
    <div className="space-y-4">
      <label className="flex items-center gap-3 text-sm text-stone-700">
        <Checkbox
          checked={Boolean(config?.ai_review?.enabled)}
          onCheckedChange={(c) => setAIReviewField("enabled", Boolean(c))}
        />
        启用 AI 审核
      </label>
      <p className="text-xs leading-6 text-stone-500">
        开启后会在请求进入生图账号前先调用审核模型，审核不通过会直接拒绝，减少违规提示词触达账号造成风控或封号的风险。
      </p>
      <div className="grid gap-4 md:grid-cols-3">
        <div className="space-y-2">
          <label className={LABEL_CLASS}>Base URL</label>
          <Input
            value={String(config?.ai_review?.base_url || "")}
            onChange={(e) => setAIReviewField("base_url", e.target.value)}
            placeholder="https://api.openai.com"
            className={INPUT_CLASS}
          />
        </div>
        <div className="space-y-2">
          <label className={LABEL_CLASS}>API Key</label>
          <Input
            value={String(config?.ai_review?.api_key || "")}
            onChange={(e) => setAIReviewField("api_key", e.target.value)}
            placeholder="sk-..."
            className={INPUT_CLASS}
          />
        </div>
        <div className="space-y-2">
          <label className={LABEL_CLASS}>Model</label>
          <Input
            value={String(config?.ai_review?.model || "")}
            onChange={(e) => setAIReviewField("model", e.target.value)}
            placeholder="gpt-4o-mini"
            className={INPUT_CLASS}
          />
        </div>
      </div>
      <div className="space-y-2">
        <label className={LABEL_CLASS}>审核提示词</label>
        <Textarea
          value={String(config?.ai_review?.prompt || "")}
          onChange={(e) => setAIReviewField("prompt", e.target.value)}
          placeholder="判断用户请求是否允许。只回答 ALLOW 或 REJECT。"
          className="min-h-24 rounded-xl border-stone-200 bg-white text-xs shadow-none"
        />
      </div>
    </div>
  );
}

/* ───────────────────────── 日志 ───────────────────────── */

export function LogSection() {
  const config = useSettingsStore((s) => s.config);
  const setLogLevel = useSettingsStore((s) => s.setLogLevel);
  const logLevelOptions = ["debug", "info", "warning", "error"];

  return (
    <div className="space-y-3">
      <label className={LABEL_CLASS}>控制台日志级别</label>
      <p className={HELP_CLASS}>不选择时使用默认 info / warning / error。</p>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {logLevelOptions.map((level) => (
          <label
            key={level}
            className={`flex items-center gap-2 ${TILE_CLASS} text-sm capitalize text-stone-700`}
          >
            <Checkbox
              checked={Boolean(config?.log_levels?.includes(level))}
              onCheckedChange={(c) => setLogLevel(level, Boolean(c))}
            />
            {level}
          </label>
        ))}
      </div>
    </div>
  );
}
