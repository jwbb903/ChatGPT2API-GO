"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Copy,
  ExternalLink,
  EyeOff,
  Image as ImageIcon,
  LoaderCircle,
  RefreshCw,
  Sparkles,
  Trash2,
  Wand2,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  fetchGalleryFeed,
  hideGalleryItem,
  unhideGalleryItem,
  unpublishGalleryItem,
  type GalleryItem,
} from "@/lib/api";
import { useAuthGuard } from "@/lib/use-auth-guard";
import { cn } from "@/lib/utils";

const PAGE_LIMIT = 24;
// 与 /works 页保持一致的 sessionStorage key，/image 页 mount 时统一消费
const REDRAW_HANDOFF_KEY = "chatgpt2api:redraw_handoff";

/**
 * 响应式断点：[最小宽度 px, 该宽度下的列数]，从大到小排，先匹配先用。
 * 抄 Pinterest 的密度档：mobile 2 列起步，4K 屏 6 列。
 */
const COL_BREAKPOINTS: Array<[number, number]> = [
  [1536, 6],
  [1280, 5],
  [1024, 4],
  [768, 3],
  [0, 2],
];

function pickColCount(width: number): number {
  for (const [min, cols] of COL_BREAKPOINTS) {
    if (width >= min) return cols;
  }
  return 2;
}

/**
 * 按"当前列累计高度最矮"分桶 = Pinterest 真瀑布流算法。
 * 我们没有真实渲染高度，但 GalleryItem 自带 width/height（后端发布时落库），
 * 用 1/ratio 作为相对单位高度估算就够了——所有卡都按等宽列渲染，
 * 同样的 ratio 误差对所有卡均匀放大，比 round-robin (i % cols) 准很多。
 *
 * 没有 width/height 的旧条目按 1:1 兜底，不影响整体平衡。
 */
function distributeMasonry(items: GalleryItem[], cols: number): GalleryItem[][] {
  const buckets: GalleryItem[][] = Array.from({ length: cols }, () => []);
  const heights: number[] = Array(cols).fill(0);
  for (const item of items) {
    const ratio =
      item.width > 0 && item.height > 0 ? item.width / item.height : 1;
    const h = 1 / ratio;
    let minIdx = 0;
    for (let i = 1; i < cols; i++) {
      if (heights[i] < heights[minIdx]) minIdx = i;
    }
    buckets[minIdx].push(item);
    heights[minIdx] += h;
  }
  return buckets;
}

function formatRelativeTime(epochSeconds: number): string {
  if (!epochSeconds) return "";
  const diff = Date.now() / 1000 - epochSeconds;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  if (diff < 86400 * 7) return `${Math.floor(diff / 86400)} 天前`;
  const date = new Date(epochSeconds * 1000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

function GalleryPageContent({ isAdmin }: { isAdmin: boolean }) {
  const [items, setItems] = useState<GalleryItem[]>([]);
  const [cursor, setCursor] = useState<string>("");
  const [hasMore, setHasMore] = useState(true);
  const [isLoading, setIsLoading] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  // admin 才能用：是否在 feed 里包含已下架（hidden）的条目
  const [includeHidden, setIncludeHidden] = useState(false);
  // 详情 dialog 焦点
  const [focused, setFocused] = useState<GalleryItem | null>(null);
  // 二次确认删除（hard unpublish）
  const [pendingDelete, setPendingDelete] = useState<GalleryItem | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);

  // 当前列数：监听窗口宽度变化挑断点。SSR 期 window 不存在先给 2 列，
  // 客户端 mount 后立即根据真实宽度修正。
  const [colCount, setColCount] = useState<number>(2);
  useEffect(() => {
    if (typeof window === "undefined") return;
    const update = () => setColCount(pickColCount(window.innerWidth));
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, []);

  const loadFirstPage = useCallback(async () => {
    setIsLoading(true);
    try {
      const resp = await fetchGalleryFeed({ limit: PAGE_LIMIT, includeHidden });
      setItems(resp.items);
      setCursor(resp.next_cursor || "");
      setHasMore(Boolean(resp.next_cursor));
    } catch (error) {
      const message = error instanceof Error ? error.message : "加载画廊失败";
      toast.error(message);
    } finally {
      setIsLoading(false);
    }
  }, [includeHidden]);

  const loadMore = useCallback(async () => {
    if (isLoadingMore || !hasMore || !cursor) return;
    setIsLoadingMore(true);
    try {
      const resp = await fetchGalleryFeed({
        cursor,
        limit: PAGE_LIMIT,
        includeHidden,
      });
      setItems((prev) => {
        const seen = new Set(prev.map((it) => it.id));
        const next = resp.items.filter((it) => !seen.has(it.id));
        return [...prev, ...next];
      });
      setCursor(resp.next_cursor || "");
      setHasMore(Boolean(resp.next_cursor));
    } catch (error) {
      const message = error instanceof Error ? error.message : "加载下一页失败";
      toast.error(message);
    } finally {
      setIsLoadingMore(false);
    }
  }, [cursor, hasMore, includeHidden, isLoadingMore]);

  useEffect(() => {
    void loadFirstPage();
  }, [loadFirstPage]);

  // IntersectionObserver 触发 loadMore：sentinel 进视口就拉下一页
  useEffect(() => {
    const node = sentinelRef.current;
    if (!node) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          void loadMore();
        }
      },
      { rootMargin: "200px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [loadMore]);

  const handleCopyPrompt = async (text: string) => {
    if (!text.trim()) return;
    try {
      await navigator.clipboard.writeText(text);
      toast.success("已复制 prompt");
    } catch {
      toast.error("复制失败");
    }
  };

  /**
   * 用此图二创：把画廊条目的 rel + url + prompt 写进 sessionStorage，跳到画图页。
   * 复用 /works 页"用此图重画"那条 handoff 链路：/image 页 mount 时统一消费这个 key。
   * - 优先传 image_rel：/image 页会拼 `/images/<rel>` 同源 fetch，不撞 CORS
   * - url 兜底：rel 缺失时用绝对地址，<img> 至少能加载（fetch 可能拦）
   * - prompt 可空：画廊本身允许空 prompt 发布，到画图页留空让用户自己写就行
   */
  const handleRedraw = (item: GalleryItem) => {
    if (typeof window === "undefined") return;
    try {
      window.sessionStorage.setItem(
        REDRAW_HANDOFF_KEY,
        JSON.stringify({
          rel: item.image_rel || "",
          url: item.url,
          prompt: item.prompt || "",
        }),
      );
    } catch {
      // 隐私模式 / 配额满时写不进去也不阻断跳转，画图页自己会兜底
    }
    window.location.assign("/image");
  };

  const handleAdminToggleHide = async (item: GalleryItem) => {
    try {
      if (item.status === "hidden") {
        await unhideGalleryItem(item.id);
        toast.success("已恢复显示");
      } else {
        await hideGalleryItem(item.id);
        toast.success("已下架");
      }
      // 局部更新比 reload 体验好，避免 cursor 重置滚动跳回顶
      setItems((prev) =>
        prev.map((it) =>
          it.id === item.id
            ? { ...it, status: it.status === "hidden" ? "visible" : "hidden" }
            : it,
        ),
      );
      setFocused((cur) =>
        cur?.id === item.id
          ? { ...cur, status: cur.status === "hidden" ? "visible" : "hidden" }
          : cur,
      );
    } catch (error) {
      const message = error instanceof Error ? error.message : "操作失败";
      toast.error(message);
    }
  };

  const handleConfirmDelete = async () => {
    if (!pendingDelete) return;
    const target = pendingDelete;
    setPendingDelete(null);
    try {
      await unpublishGalleryItem(target.id);
      setItems((prev) => prev.filter((it) => it.id !== target.id));
      setFocused((cur) => (cur?.id === target.id ? null : cur));
      toast.success("已删除");
    } catch (error) {
      const message = error instanceof Error ? error.message : "删除失败";
      toast.error(message);
    }
  };

  /**
   * 发布者本人撤回。后端 unpublish 路由本身就支持本人撤回（is_admin 分支之外
   * 单独有 publisher_id == requester_id 校验），所以这里直接复用同一个 API。
   * 与 admin 的「永久删除」走的同一接口同一语义——撤回 = 从画廊里把这条记录删了，
   * 但原图（image_owners）不会动，作品在「我的作品」里仍保留。
   */
  const handleSelfUnpublish = async (item: GalleryItem) => {
    try {
      await unpublishGalleryItem(item.id);
      setItems((prev) => prev.filter((it) => it.id !== item.id));
      setFocused((cur) => (cur?.id === item.id ? null : cur));
      toast.success("已撤回发布");
    } catch (error) {
      const message = error instanceof Error ? error.message : "撤回失败";
      toast.error(message);
    }
  };

  const visibleCount = useMemo(
    () => items.filter((it) => it.status === "visible").length,
    [items],
  );

  // 关闭弹窗时，focused 立刻置 null 会让 {focused ? ... : null} 内容瞬间从 DOM 消失，
  // 剩下空的 DialogContent 在 Radix 淡出动画里收缩成"屏幕中间一条白线"。
  // 用 lastFocused 缓存最后一次的内容，关闭过渡跑完前继续渲染同一份图片/按钮，
  // 整块跟外壳一起淡出。works 页同款做法。
  const [lastFocused, setLastFocused] = useState<GalleryItem | null>(null);
  useEffect(() => {
    if (focused) setLastFocused(focused);
  }, [focused]);
  const focusedView = focused ?? lastFocused;

  return (
    <>
      <section className="mt-4 flex flex-col gap-4 sm:mt-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-1">
          <div className="text-xs font-semibold tracking-[0.18em] text-stone-500 uppercase">
            Public Gallery
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">公共画廊</h1>
          <p className="text-sm text-muted-foreground">
            {isLoading
              ? "正在加载…"
              : items.length === 0
                ? "还没有人发布作品"
                : `共 ${visibleCount} 张可见 · 点击卡片查看 prompt`}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {isAdmin ? (
            <label className="inline-flex cursor-pointer items-center gap-2 rounded-xl border border-stone-200 bg-white/80 px-3 py-2 text-sm text-stone-700 hover:bg-white">
              <Checkbox
                checked={includeHidden}
                onCheckedChange={(v) => setIncludeHidden(Boolean(v))}
              />
              <span>显示已下架</span>
            </label>
          ) : null}
          <Button
            variant="outline"
            className="h-10 rounded-xl border-stone-200 bg-white/80 px-4 text-stone-700 hover:bg-white"
            onClick={() => void loadFirstPage()}
            disabled={isLoading}
          >
            <RefreshCw className={cn("size-4", isLoading && "animate-spin")} />
            刷新
          </Button>
        </div>
      </section>

      {isLoading && items.length === 0 ? (
        <Card className="mt-6 rounded-2xl border-white/80 bg-white/90 shadow-sm">
          <CardContent className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
            <div className="rounded-xl bg-stone-100 p-3 text-stone-500">
              <LoaderCircle className="size-5 animate-spin" />
            </div>
            <p className="text-sm text-stone-500">从画廊同步作品…</p>
          </CardContent>
        </Card>
      ) : null}

      {!isLoading && items.length === 0 ? (
        <Card className="mt-6 rounded-2xl border-white/80 bg-white/90 shadow-sm">
          <CardContent className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
            <div className="rounded-xl bg-stone-100 p-3 text-stone-500">
              <ImageIcon className="size-5" />
            </div>
            <div className="space-y-1">
              <p className="text-sm font-medium text-stone-700">画廊还很空</p>
              <p className="text-sm text-stone-500">
                到「我的作品」打开任意作品，点「发布到画廊」即可分享
              </p>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {/* 真瀑布流：手动按"当前列累计高度最矮"分桶。
          CSS columns + column-fill: balance 在内容少时浏览器会平衡两列高度，
          俩条目时直接塞到第一列，不是 bug 是规范——所以瀑布流必须自己分。
          每列内部用 flex-col 顺序堆，列间 gap-3 在 wrapper 上控。 */}
      <div className="mt-6 flex gap-3">
        {distributeMasonry(items, colCount).map((bucket, colIdx) => (
          <div key={colIdx} className="flex flex-1 flex-col gap-3">
            {bucket.map((item) => {
              const ratio =
                item.width > 0 && item.height > 0
                  ? item.width / item.height
                  : 1;
              const isHidden = item.status === "hidden";
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => setFocused(item)}
                  className={cn(
                    "group relative w-full cursor-pointer overflow-hidden rounded-2xl border border-stone-200/80 bg-stone-100 text-left shadow-sm transition hover:shadow-md",
                    isHidden && "opacity-60 hover:opacity-90",
                  )}
                  style={{ aspectRatio: String(ratio) }}
                >
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={item.url}
                    alt={item.prompt.slice(0, 30) || "作品"}
                    loading="lazy"
                    className="h-full w-full object-cover transition group-hover:scale-[1.02]"
                  />
                  {isHidden ? (
                    <div className="absolute top-2 left-2 rounded-md bg-rose-500/90 px-2 py-1 text-[10.5px] font-semibold text-white">
                      已下架
                    </div>
                  ) : null}
                  {/* 图生图角标：右上避开「已下架」的左上位置。
                      纯文生图卡片不显示，避免普通用户视觉负担。 */}
                  {item.is_edit ? (
                    <div className="pointer-events-none absolute top-2 right-2 inline-flex items-center gap-1 rounded-md bg-amber-500/95 px-1.5 py-0.5 text-[10px] font-semibold text-white shadow-sm">
                      <Wand2 className="size-3" />
                      图生图
                    </div>
                  ) : null}
                  {/* hover 时才浮现作者/时间，平时纯图——抄 Pinterest 静态密集铺面的体感 */}
                  <div className="pointer-events-none absolute right-0 bottom-0 left-0 flex items-end justify-between gap-2 bg-gradient-to-t from-black/60 to-transparent p-2 text-[10.5px] text-white/90 opacity-0 transition group-hover:opacity-100">
                    <span className="truncate">{item.publisher_name || "匿名"}</span>
                    <span className="shrink-0">{formatRelativeTime(item.created_at)}</span>
                  </div>
                </button>
              );
            })}
          </div>
        ))}
      </div>

      {/* 滚到底触发器 */}
      <div ref={sentinelRef} className="mt-6 flex h-10 items-center justify-center text-xs text-stone-400">
        {isLoadingMore ? (
          <span className="inline-flex items-center gap-2">
            <LoaderCircle className="size-3 animate-spin" />
            加载中…
          </span>
        ) : hasMore ? (
          "下滑加载更多"
        ) : items.length > 0 ? (
          "已经到底了"
        ) : null}
      </div>

      {/* 详情 Dialog */}
      <Dialog open={focused !== null} onOpenChange={(open) => (!open ? setFocused(null) : null)}>
        <DialogContent
          showCloseButton={false}
          className="hide-scrollbar max-h-[92vh] overflow-y-auto rounded-2xl p-0 sm:max-w-[760px]"
        >
          {focusedView ? (
            <div className="flex flex-col">
              {/* 图片容器：iOS Photos / Spotify 同款"自模糊铺底"——
                  - 把同一张图作为 background-image cover 铺整个容器
                  - 上面叠一层 backdrop-blur + 半透明 dim 拉糊到只剩色块氛围
                  - 实际图片用 object-contain 居中，max-h 限到 65vh 让弹窗一屏装得下
                  竖图两侧的"留白"变成原图的模糊延伸光，不再是突兀的纯黑/纯灰。 */}
              <div
                className="relative overflow-hidden bg-stone-200"
                style={{
                  backgroundImage: `url(${focusedView.url})`,
                  backgroundSize: "cover",
                  backgroundPosition: "center",
                }}
              >
                <div className="absolute inset-0 bg-stone-950/35 backdrop-blur-2xl" />
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={focusedView.url}
                  alt={focusedView.prompt.slice(0, 30) || "作品"}
                  className="relative mx-auto block h-auto max-h-[65vh] w-full object-contain"
                />
              </div>
              <div className="flex flex-col gap-3 p-5">
                <DialogHeader className="gap-1.5 space-y-0">
                  <DialogTitle className="text-base font-semibold">
                    {focusedView.is_edit ? "图生图作品" : "Prompt"}
                  </DialogTitle>
                  <DialogDescription className="sr-only">作品详情</DialogDescription>
                </DialogHeader>
                {/* 图生图：prompt 是相对参考图的修改指令（"换个浅色版"、"加帽子"），
                    后端 publish 时已强制把 prompt 落空。前端展示成琥珀色提示卡，
                    告诉看图的人这段提示词无法独立复用，免得复制了一段抽象指令
                    回去发现完全跑偏。复制按钮也会跟着 disabled。 */}
                {focusedView.is_edit ? (
                  <div className="flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 p-3 text-[12.5px] leading-6 text-amber-900">
                    <Wand2 className="mt-0.5 size-4 shrink-0 text-amber-600" />
                    <span>
                      这是图生图作品，提示词依赖原始参考图，无法独立复用。点击「用此图二创」可以把这张图当参考图继续创作。
                    </span>
                  </div>
                ) : (
                  <div className="rounded-xl bg-stone-50 p-3 text-[13px] leading-6 text-stone-800">
                    {focusedView.prompt || "—"}
                  </div>
                )}
                <div className="flex flex-wrap items-center gap-2 text-xs text-stone-500">
                  {focusedView.model ? (
                    <Badge variant="secondary" className="rounded-md font-medium">
                      {focusedView.model}
                    </Badge>
                  ) : null}
                  {focusedView.size ? (
                    <Badge variant="secondary" className="rounded-md font-medium">
                      {focusedView.size}
                    </Badge>
                  ) : null}
                  <span>· {focusedView.publisher_name || "匿名"}</span>
                  <span>· {formatRelativeTime(focusedView.created_at)}</span>
                  {focusedView.status === "hidden" ? (
                    <Badge className="rounded-md bg-rose-500 text-white">已下架</Badge>
                  ) : null}
                </div>

                {/* 主排：所有人都能用的 3 个 CTA，强 grid 等分永远不换行。
                    复制 / 二创 / 查看原图——查看原图作为通用次要 action 留在主排，
                    避免管理排在普通 user 视角下空着一排。 */}
                <div className="mt-2 grid grid-cols-3 gap-2">
                  <Button
                    onClick={() => void handleCopyPrompt(focusedView.prompt)}
                    disabled={focusedView.is_edit || !focusedView.prompt?.trim()}
                    className="h-10 w-full rounded-xl bg-stone-950 px-2 text-white hover:bg-stone-800 disabled:cursor-not-allowed disabled:bg-stone-300 disabled:text-stone-500 disabled:hover:bg-stone-300"
                  >
                    <Copy className="size-4" />
                    复制 prompt
                  </Button>
                  <Button
                    onClick={() => handleRedraw(focusedView)}
                    className="h-10 w-full rounded-xl bg-stone-950 px-2 text-white hover:bg-stone-800"
                  >
                    <Wand2 className="size-4" />
                    用此图二创
                  </Button>
                  <Button
                    variant="outline"
                    className="h-10 w-full rounded-xl border-stone-200 bg-white px-2"
                    onClick={() => window.open(focusedView.url, "_blank", "noopener,noreferrer")}
                  >
                    <ExternalLink className="size-4" />
                    查看原图
                  </Button>
                </div>
                {/* 管理排：撤回（is_mine）/ 下架（admin）/ 永久删除（admin）。
                    动态算列数 = 命中条件个数，让 1/2/3 个按钮都能均分宽度，
                    既不被压扁也不会出现"主排 4 + 次排 1 独占"的视觉断层。 */}
                {(() => {
                  const showSelfUnpublish = focusedView.is_mine;
                  const showAdminHide = isAdmin;
                  const showAdminDelete = isAdmin;
                  const cols = (showSelfUnpublish ? 1 : 0) + (showAdminHide ? 1 : 0) + (showAdminDelete ? 1 : 0);
                  if (cols === 0) return null;
                  const gridClass =
                    cols === 1 ? "grid-cols-1" : cols === 2 ? "grid-cols-2" : "grid-cols-3";
                  return (
                    <div className={`grid ${gridClass} gap-2`}>
                      {showSelfUnpublish ? (
                        <Button
                          variant="outline"
                          className="h-10 w-full rounded-xl border-rose-200 bg-white px-2 text-rose-600 hover:bg-rose-50"
                          onClick={() => void handleSelfUnpublish(focusedView)}
                        >
                          <Trash2 className="size-4" />
                          撤回发布
                        </Button>
                      ) : null}
                      {showAdminHide ? (
                        <Button
                          variant="outline"
                          className="h-10 w-full rounded-xl border-stone-200 bg-white px-2"
                          onClick={() => void handleAdminToggleHide(focusedView)}
                        >
                          <EyeOff className="size-4" />
                          {focusedView.status === "hidden" ? "恢复显示" : "下架"}
                        </Button>
                      ) : null}
                      {showAdminDelete ? (
                        <Button
                          className="h-10 w-full rounded-xl bg-rose-600 px-2 text-white hover:bg-rose-700"
                          onClick={() => setPendingDelete(focusedView)}
                        >
                          <Trash2 className="size-4" />
                          永久删除
                        </Button>
                      ) : null}
                    </div>
                  );
                })()}
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      {/* 永久删除二次确认（仅 admin 触发） */}
      <Dialog
        open={pendingDelete !== null}
        onOpenChange={(open) => (!open ? setPendingDelete(null) : null)}
      >
        <DialogContent showCloseButton={false} className="rounded-2xl p-6">
          <DialogHeader>
            <DialogTitle>永久删除画廊条目？</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              这只会从画廊移除，不会删除原图（发布者本人的「我的作品」仍保留这张）。如果只想暂时隐藏，请用「下架」。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)}>
              取消
            </Button>
            <Button
              className="bg-rose-600 text-white hover:bg-rose-700"
              onClick={() => void handleConfirmDelete()}
            >
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 发布入口提示 */}
      <div className="mt-12 flex items-center justify-center gap-2 text-xs text-stone-400">
        <Sparkles className="size-3" />
        <span>发布入口：「我的作品」页打开任意作品 → 发布到画廊</span>
      </div>
    </>
  );
}

export default function GalleryPage() {
  const { isCheckingAuth, session } = useAuthGuard();

  if (isCheckingAuth || !session) {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <LoaderCircle className="size-5 animate-spin text-stone-400" />
      </div>
    );
  }

  return <GalleryPageContent isAdmin={session.role === "admin"} />;
}
