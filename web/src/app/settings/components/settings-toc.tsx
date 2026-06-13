"use client";

import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";

/**
 * 设置页右锚 TOC（table of contents）。
 *
 * 行为：
 *   - sticky 钉在右侧，lg+ 才显示——< 1024px 直接隐藏，避免压窄主内容区
 *   - 用 IntersectionObserver 监听各 [data-settings-section] 节点，
 *     选 viewport 顶部之下、最靠上的那个 section 作为 active
 *   - 点击 TOC item 用 scrollIntoView({ behavior: "smooth", block: "start" })
 *     滚到对应 section；section 自带 scroll-mt-24 给 sticky header 让位
 *
 * TOC items 由父组件传入，避免硬编码顺序——以后调换 section 顺序不用动 TOC。
 */
export type TOCItem = { id: string; label: string };

export function SettingsTOC({ items }: { items: TOCItem[] }) {
  const [activeId, setActiveId] = useState<string>(items[0]?.id ?? "");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const targets = items
      .map((it) => document.querySelector(`[data-settings-section="${it.id}"]`))
      .filter((el): el is Element => Boolean(el));
    if (targets.length === 0) return;

    // rootMargin 上 -20% 下 -70%：把"激活区"压到视口顶部 ~20% 处的窄带，
    // 滚动时同一时刻只命中一个 section，TOC 不会闪烁切换
    const observer = new IntersectionObserver(
      (entries) => {
        // 拿到当前所有相交且最靠上的那个
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible.length > 0) {
          const id = visible[0].target.getAttribute("data-settings-section");
          if (id) setActiveId(id);
        }
      },
      { rootMargin: "-20% 0px -70% 0px", threshold: 0 },
    );
    targets.forEach((t) => observer.observe(t));
    return () => observer.disconnect();
  }, [items]);

  const handleClick = (id: string) => {
    const el = document.querySelector(`[data-settings-section="${id}"]`);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
      // 立刻更新高亮，不等 IO 回调，避免点击和高亮之间有 ~200ms 延迟
      setActiveId(id);
    }
  };

  return (
    <aside className="sticky top-24 hidden h-fit w-56 shrink-0 lg:block">
      <div className="text-xs font-semibold tracking-[0.18em] text-stone-500 uppercase">
        On this page
      </div>
      <nav className="mt-3 flex flex-col gap-0.5">
        {items.map((it) => {
          const active = it.id === activeId;
          return (
            <button
              key={it.id}
              type="button"
              onClick={() => handleClick(it.id)}
              className={cn(
                "group relative cursor-pointer rounded-md px-3 py-1.5 text-left text-sm transition-colors duration-150",
                "border-l-2",
                active
                  ? "border-stone-900 bg-stone-100/70 font-medium text-stone-900"
                  : "border-transparent text-stone-500 hover:bg-stone-100/50 hover:text-stone-800",
              )}
            >
              {it.label}
            </button>
          );
        })}
      </nav>
    </aside>
  );
}
