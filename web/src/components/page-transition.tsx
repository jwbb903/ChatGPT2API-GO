"use client";

import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

/**
 * 路由切换时给页面内容做轻量淡入。
 *
 * 实现：用 pathname 作为 key 强制 React 在路径变化时丢掉旧子树、
 * 挂上新子树，新子树挂载瞬间触发 CSS animation。
 *
 * 不会破坏任何 fixed 定位元素（TopNav / RouteProgress / Toaster
 * 都在这个容器之外），也不会影响 main 的滚动条 gutter。
 */
export function PageTransition({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return (
    <div key={pathname} className="animate-page-enter">
      {children}
    </div>
  );
}
