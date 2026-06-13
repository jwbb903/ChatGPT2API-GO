"use client";

import { usePathname } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

/**
 * 顶部路由进度条 (Vercel / GitHub / YouTube 同款)。
 *
 * 工作原理：
 *  - 监听全局 click，命中同源 <a> 链接（且目标 URL 与当前不同）时启动；
 *  - 监听 popstate（浏览器前进/后退）时启动；
 *  - 启动后用 setInterval 缓慢向 90% 蠕动；
 *  - usePathname 变化时收尾到 100% 然后淡出；
 *  - 设置最小可见时长，避免本地路由切换过快导致进度条只闪一下看不到。
 *
 * 不依赖第三方包，跟随 --primary 配色 + 蓝色光晕。
 */
const MIN_VISIBLE_MS = 400;
const START_PROGRESS = 25;

export function RouteProgress() {
  const pathname = usePathname();
  const [progress, setProgress] = useState(0);
  const [visible, setVisible] = useState(false);

  const trickleTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const fadeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const finishTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const runningRef = useRef(false);
  const startedAtRef = useRef(0);
  const lastPathRef = useRef(pathname);

  const clearAll = useCallback(() => {
    if (trickleTimer.current) {
      clearInterval(trickleTimer.current);
      trickleTimer.current = null;
    }
    if (fadeTimer.current) {
      clearTimeout(fadeTimer.current);
      fadeTimer.current = null;
    }
    if (finishTimer.current) {
      clearTimeout(finishTimer.current);
      finishTimer.current = null;
    }
    if (resetTimer.current) {
      clearTimeout(resetTimer.current);
      resetTimer.current = null;
    }
  }, []);

  const start = useCallback(() => {
    if (runningRef.current) return;
    runningRef.current = true;
    startedAtRef.current = Date.now();
    clearAll();
    setVisible(true);
    setProgress(START_PROGRESS);
    trickleTimer.current = setInterval(() => {
      setProgress((prev) => (prev >= 90 ? prev : prev + (90 - prev) * 0.12));
    }, 180);
  }, [clearAll]);

  const finish = useCallback(() => {
    if (!runningRef.current) return;
    const elapsed = Date.now() - startedAtRef.current;
    const wait = Math.max(0, MIN_VISIBLE_MS - elapsed);

    if (finishTimer.current) {
      clearTimeout(finishTimer.current);
    }
    finishTimer.current = setTimeout(() => {
      runningRef.current = false;
      if (trickleTimer.current) {
        clearInterval(trickleTimer.current);
        trickleTimer.current = null;
      }
      setProgress(100);
      fadeTimer.current = setTimeout(() => {
        setVisible(false);
        // 进度条彻底淡出后再回到 0，避免下一次启动时出现 100→0 的回拉
        resetTimer.current = setTimeout(() => setProgress(0), 240);
      }, 220);
    }, wait);
  }, []);

  // 同源链接点击时启动。
  // 注意：Next.js <Link> 内部会调 preventDefault 走客户端路由，
  // 用 capture 阶段抢在 React 委托 handler 之前拿到事件，
  // 否则 event.defaultPrevented 已经是 true，逻辑会被绕开。
  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      if (event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;

      const target = event.target as Element | null;
      const anchor = target?.closest?.("a") as HTMLAnchorElement | null;
      if (!anchor) return;

      const targetAttr = anchor.getAttribute("target");
      if (targetAttr && targetAttr !== "_self") return;

      const href = anchor.getAttribute("href");
      if (!href) return;
      if (
        href.startsWith("mailto:") ||
        href.startsWith("tel:") ||
        href.startsWith("#") ||
        anchor.hasAttribute("download")
      ) {
        return;
      }

      let url: URL;
      try {
        url = new URL(anchor.href, window.location.href);
      } catch {
        return;
      }
      if (url.origin !== window.location.origin) return;
      if (url.pathname === window.location.pathname && url.search === window.location.search) {
        return;
      }

      start();
    };

    document.addEventListener("click", onClick, true);
    return () => document.removeEventListener("click", onClick, true);
  }, [start]);

  // 浏览器前进/后退
  useEffect(() => {
    const onPopState = () => start();
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, [start]);

  // 路径变化时收尾
  useEffect(() => {
    if (lastPathRef.current !== pathname) {
      lastPathRef.current = pathname;
      finish();
    }
  }, [pathname, finish]);

  // 卸载清理
  useEffect(() => clearAll, [clearAll]);

  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-x-0 top-0 z-[100] h-[2px]"
      style={{
        opacity: visible ? 1 : 0,
        transition: visible ? "opacity 80ms linear" : "opacity 220ms 80ms linear",
      }}
    >
      <div
        className="h-full"
        style={{
          width: `${progress}%`,
          backgroundColor: "oklch(0.7 0.13 250)",
          transition:
            progress === 0
              ? "none"
              : progress >= 100
                ? "width 220ms ease-out"
                : "width 220ms cubic-bezier(0.22, 0.61, 0.36, 1)",
          boxShadow:
            progress > 0
              ? "0 0 10px oklch(0.7 0.13 250 / 0.55), 0 0 4px oklch(0.7 0.13 250 / 0.5)"
              : undefined,
        }}
      />
    </div>
  );
}
