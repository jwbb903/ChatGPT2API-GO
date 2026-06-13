"use client";

import { ReactNode } from "react";

/**
 * 设置页通用 section 包装器：
 *   - id 用作 URL hash 锚点 + TOC 跳转目标 + IntersectionObserver 监听单元
 *   - title / description 走统一字号节奏（text-xl / text-sm muted）
 *   - 不再每节套一张 Card——单页太多 Card 反而稀释层级；
 *     用 border-t + 间距分隔即可，跟 Linear / Vercel settings 一样轻
 *   - scroll-mt-24：sticky header 让位，scrollIntoView 时不会被顶栏盖住
 */
export function Section({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section
      id={id}
      data-settings-section={id}
      className="scroll-mt-24 space-y-6 border-t border-stone-200/80 pt-10 first:border-t-0 first:pt-0"
    >
      <header className="space-y-1">
        <h2 className="text-xl font-semibold tracking-tight text-stone-900">{title}</h2>
        {description ? (
          <p className="text-sm leading-6 text-stone-500">{description}</p>
        ) : null}
      </header>
      <div>{children}</div>
    </section>
  );
}
