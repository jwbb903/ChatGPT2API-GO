"use client";

import { useEffect, useRef } from "react";
import { LoaderCircle } from "lucide-react";

import { useAuthGuard } from "@/lib/use-auth-guard";

import { FloatingSaveBar } from "./components/floating-save-bar";
import { Section } from "./components/section";
import { SettingsHeader } from "./components/settings-header";
import { SettingsTOC, type TOCItem } from "./components/settings-toc";
import {
  AccountSection,
  AIReviewSection,
  ImageSection,
  LogSection,
  NetworkSection,
  SecuritySection,
} from "./components/settings-sections";
import { useSettingsStore } from "./store";

/**
 * TOC 顺序 = 页面 section 顺序，不需要双份维护：
 *   - 主内容区 map 这条 list 渲染 <Section>
 *   - 右侧 TOC 也用这条 list
 */
const SECTIONS: Array<TOCItem & { description: string }> = [
  { id: "account", label: "账号与身份", description: "账号刷新策略与自动维护开关。用户密钥分发请前往「用户密钥」页。" },
  { id: "network", label: "网络", description: "全局代理：同时影响生图请求和 OpenAI 上游转发。" },
  { id: "images", label: "图片", description: "访问地址、生成超时、并发上限、过期清理及保护策略。" },
  { id: "security", label: "内容安全", description: "敏感词与全局附加指令——把审查放在请求落到生图账号之前。" },
  { id: "ai-review", label: "AI 审核", description: "用一个独立模型对用户提示词做合规判断，命中即拒绝。" },
  { id: "logs", label: "日志", description: "控制台输出级别。debug 仅排查问题时打开。" },
];

function SettingsDataController() {
  const didLoadRef = useRef(false);
  const initialize = useSettingsStore((state) => state.initialize);

  useEffect(() => {
    if (didLoadRef.current) return;
    didLoadRef.current = true;
    void initialize();
  }, [initialize]);

  return null;
}

/**
 * Section 内容路由：根据 id 渲染对应组件。
 * 把映射放这里而不是 SECTIONS 数组里，是因为 sections 数据要序列化传给 TOC，
 * 不能塞 React 组件。
 */
function SectionBody({ id }: { id: string }) {
  switch (id) {
    case "account":
      return <AccountSection />;
    case "network":
      return <NetworkSection />;
    case "images":
      return <ImageSection />;
    case "security":
      return <SecuritySection />;
    case "ai-review":
      return <AIReviewSection />;
    case "logs":
      return <LogSection />;
    default:
      return null;
  }
}

function SettingsPageContent() {
  const tocItems: TOCItem[] = SECTIONS.map(({ id, label }) => ({ id, label }));
  return (
    <>
      <SettingsDataController />
      <SettingsHeader />

      {/* 左主内容 + 右锚 TOC：lg+ 用 grid 双栏，移动端 TOC 由自身的 hidden lg:block 隐藏。
          gap-12：让主内容区与 TOC 之间留出充足的"空气"，TOC 不会贴着内容卡边缘。
          pb-24：给底部 FloatingSaveBar 留位，避免它浮现时盖住最后一条 section 的输入。 */}
      <div className="mt-8 flex gap-12 pb-24">
        <main className="min-w-0 flex-1 space-y-12">
          {SECTIONS.map(({ id, label, description }) => (
            <Section key={id} id={id} title={label} description={description}>
              <SectionBody id={id} />
            </Section>
          ))}
        </main>
        <SettingsTOC items={tocItems} />
      </div>

      <FloatingSaveBar />
    </>
  );
}

export default function SettingsPage() {
  const { isCheckingAuth, session } = useAuthGuard(["admin"]);

  if (isCheckingAuth || !session || session.role !== "admin") {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <LoaderCircle className="size-5 animate-spin text-stone-400" />
      </div>
    );
  }

  return <SettingsPageContent />;
}
