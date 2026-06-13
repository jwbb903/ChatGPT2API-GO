"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Copy,
  Download,
  ImageIcon,
  Images,
  LoaderCircle,
  RefreshCw,
  Share2,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import {
  deleteManagedImages,
  downloadSingleImage,
  fetchMyWorks,
  getMyPublishedBatch,
  publishGalleryItem,
  type ManagedImage,
} from "@/lib/api";
import { useAuthGuard } from "@/lib/use-auth-guard";
import { cn } from "@/lib/utils";

/**
 * sessionStorage 移交给画图页的 key。
 * 格式：{ url: string; prompt: string }
 * 画图页 mount 时读一次，立刻清掉，避免下次刷新又触发。
 */
const REDRAW_HANDOFF_KEY = "chatgpt2api:redraw_handoff";

function imageKey(item: ManagedImage) {
  return item.rel || item.url;
}

function formatRelative(value: string) {
  if (!value) return "";
  const ts = new Date(value.replace(" ", "T")).getTime();
  if (Number.isNaN(ts)) return value;
  const diff = (Date.now() - ts) / 1000;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  if (diff < 86400 * 7) return `${Math.floor(diff / 86400)} 天前`;
  return value.slice(0, 10);
}

function WorksPageContent() {
  const [items, setItems] = useState<ManagedImage[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [focused, setFocused] = useState<ManagedImage | null>(null);

  // Pinterest 风格 masonry：列宽 flex-1 边到边等分容器（不留白），列数随容器宽度走。
  //   - 列数 = round((容器宽 + gap) / (目标列宽 240 + gap))
  //   - 关键是 round 而不是 floor：floor 必须装满整数列才加新列，
  //     往往在 N+0.9 列还停在 N 列，单列特别宽 (≈1.7×目标宽)，看起来是大块卡片不是 masonry；
  //     round 在 N+0.5 列就跳到 N+1 列，单列宽稳定在 [0.7, 1.3]×目标宽，
  //     跨列数边界时单列宽只变 ~15% (不像断点 25-33% 突变那么硬)
  //   - 移动端 (<480px) 兜底 2 列，避免单列大图占满屏
  //   - 列数变化时整体过渡用 CSS transition 软化
  // ResizeObserver 监听容器，比 window.resize 更准（侧栏开合也响应）；
  // rAF 节流避免拖动时 setState 高频抖动。
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [columnCount, setColumnCount] = useState(0); // 0 = 还没测量
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const TARGET_W = 240;
    const GAP = 16;
    let raf = 0;
    const calc = () => {
      raf = 0;
      const w = el.clientWidth;
      if (!w) return;
      let n: number;
      if (w < 360) n = 1;
      else if (w < 520) n = 2;
      else n = Math.max(2, Math.round((w + GAP) / (TARGET_W + GAP)));
      setColumnCount((prev) => (prev === n ? prev : n));
    };
    const schedule = () => {
      if (raf) return;
      raf = requestAnimationFrame(calc);
    };
    schedule();
    const ro = new ResizeObserver(schedule);
    ro.observe(el);
    return () => {
      ro.disconnect();
      if (raf) cancelAnimationFrame(raf);
    };
  }, []);

  // 发布画廊弹窗：当一张图没有 prompt 时（老数据），需要让用户手填后再 publish。
  // pendingPublish 持有正在发布的目标，promptDraft 是输入框文本。
  const [pendingPublish, setPendingPublish] = useState<ManagedImage | null>(null);
  const [promptDraft, setPromptDraft] = useState("");
  const [publishing, setPublishing] = useState(false);
  // 单图当前发布态视觉反馈：rel → "publishing" | "published"
  const [publishStates, setPublishStates] = useState<Map<string, "publishing" | "published">>(
    () => new Map(),
  );

  // 删除二次确认
  const [pendingDelete, setPendingDelete] = useState<ManagedImage | null>(null);
  const [deleting, setDeleting] = useState(false);

  const reload = useCallback(async () => {
    setIsLoading(true);
    try {
      const resp = await fetchMyWorks();
      setItems(resp.items);
      // 播种 publishStates：刷新页面后 publishStates Map 会被重置为空，
      // 已发布角标会丢。reload 时一次性问后端"这批 rel 我发过哪些"，
      // 把命中的写回 state，避免逐张发单条 /api/gallery/published 撑爆并发数。
      const rels = resp.items.map((it) => it.rel).filter(Boolean) as string[];
      if (rels.length > 0) {
        try {
          const { items: published } = await getMyPublishedBatch(rels);
          setPublishStates((prev) => {
            const next = new Map(prev);
            for (const [rel, info] of Object.entries(published)) {
              if (info.published) {
                next.set(rel, "published");
              }
            }
            return next;
          });
        } catch {
          // 静默失败：拉不到发布状态不阻塞列表加载，下次 reload 再试
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "加载作品失败";
      toast.error(message);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  /**
   * 用此图重画：把 rel + prompt 写进 sessionStorage，跳到画图页。
   * 故意传 rel 不传 item.url：后端拼的 item.url 是绝对地址（含 http://...:port），
   * 跟前端页面跨源时 <img> 能加载、fetch 会被 CORS 拦掉报 "Failed to fetch"。
   * 画图页拿到 rel 后用 `/images/${rel}` 同源拉取，永远不会撞 CORS。
   * url 字段保留作为兜底（rel 缺失时的老 handoff 格式）。
   */
  const handleRedraw = useCallback((item: ManagedImage) => {
    if (typeof window === "undefined") return;
    const rel = item.rel || item.path || "";
    try {
      window.sessionStorage.setItem(
        REDRAW_HANDOFF_KEY,
        JSON.stringify({
          rel,
          url: item.url, // 兜底：rel 没拿到时用绝对地址
          prompt: item.prompt || "",
        }),
      );
    } catch {
      // sessionStorage 写失败一般是隐私模式 / 配额满，不阻断跳转
    }
    window.location.assign("/image");
  }, []);

  const handleCopyPrompt = useCallback(async (text: string) => {
    if (!text.trim()) {
      toast.error("此图没有保留 prompt");
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      toast.success("已复制 prompt");
    } catch {
      toast.error("复制失败");
    }
  }, []);

  const handleDownload = useCallback(async (item: ManagedImage) => {
    const path = item.rel || item.path;
    if (!path) {
      toast.error("当前图片无法下载");
      return;
    }
    try {
      await downloadSingleImage(path);
    } catch (error) {
      const message = error instanceof Error ? error.message : "下载失败";
      toast.error(message);
    }
  }, []);

  /**
   * 发布按钮入口。
   *  - 已有 prompt：直接走 publish 接口，过敏感词 → 成功 → 给绿对勾视觉
   *  - 没有 prompt：弹个对话框让用户手填，提交时再走 publish
   */
  const handlePublish = useCallback(
    async (item: ManagedImage, promptOverride?: string) => {
      const rel = item.rel || item.path;
      if (!rel) {
        toast.error("当前图片无法发布");
        return;
      }
      // promptOverride !== undefined 表示用户已通过补齐弹窗确认（即便是空串），
      // 此时尊重用户选择直接发布；undefined 表示从卡片入口直接点的发布按钮。
      let prompt: string;
      if (promptOverride !== undefined) {
        prompt = promptOverride.trim();
      } else {
        prompt = (item.prompt ?? "").trim();
        if (!prompt) {
          // 卡片自身没 prompt → 弹窗让用户决定加不加（可选，留空也能发）
          setPendingPublish(item);
          setPromptDraft("");
          return;
        }
      }
      setPublishStates((prev) => new Map(prev).set(rel, "publishing"));
      try {
        await publishGalleryItem({
          image_rel: rel,
          prompt,
          model: "",
          size: "",
          width: item.width || 0,
          height: item.height || 0,
        });
        setPublishStates((prev) => new Map(prev).set(rel, "published"));
        toast.success("已发布到画廊");
      } catch (error) {
        // 失败回滚状态让用户可重试
        setPublishStates((prev) => {
          const next = new Map(prev);
          next.delete(rel);
          return next;
        });
        const message = error instanceof Error ? error.message : "发布失败";
        toast.error(message);
      }
    },
    [],
  );

  const handleConfirmPendingPublish = useCallback(async () => {
    if (!pendingPublish) return;
    // 允许空 prompt——是否补齐由用户决定，后端已支持空值发布
    const text = promptDraft.trim();
    setPublishing(true);
    try {
      await handlePublish(pendingPublish, text);
      setPendingPublish(null);
      setPromptDraft("");
    } finally {
      setPublishing(false);
    }
  }, [handlePublish, pendingPublish, promptDraft]);

  const handleConfirmDelete = useCallback(async () => {
    if (!pendingDelete) return;
    const path = pendingDelete.rel || pendingDelete.path;
    if (!path) {
      setPendingDelete(null);
      return;
    }
    setDeleting(true);
    try {
      const resp = await deleteManagedImages({ paths: [path] });
      if (!resp.removed) {
        toast.error("删除失败：该图不在你名下或已不存在");
      } else {
        toast.success("已删除");
        const key = imageKey(pendingDelete);
        setItems((prev) => prev.filter((it) => imageKey(it) !== key));
        if (focused && imageKey(focused) === key) setFocused(null);
      }
      setPendingDelete(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : "删除失败";
      toast.error(message);
    } finally {
      setDeleting(false);
    }
  }, [focused, pendingDelete]);

  const visibleCount = items.length;

  // 关闭弹窗时，focused 立刻置 null 会让 {focused ? ... : null} 内容瞬间从 DOM 消失，
  // 剩下空的 DialogContent 在 Radix 200ms 淡出缩放里收缩成一条白线（用户反馈的"中间闪白线"）。
  // 用 lastFocused 缓存最后一次的内容，关闭过渡跑完前继续渲染同一份图片/按钮，
  // 整块跟着外壳一起淡出，不会先空掉。
  const [lastFocused, setLastFocused] = useState<ManagedImage | null>(null);
  useEffect(() => {
    if (focused) setLastFocused(focused);
  }, [focused]);
  const focusedView = focused ?? lastFocused;

  const focusedPublishState = focused ? publishStates.get(imageKey(focused)) : undefined;

  return (
    <>
      <section className="mt-4 flex flex-col gap-4 sm:mt-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-1">
          <div className="text-xs font-semibold tracking-[0.18em] text-stone-500 uppercase">
            My Works
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">我的作品</h1>
          <p className="text-sm text-muted-foreground">
            {isLoading
              ? "正在加载…"
              : visibleCount === 0
                ? "还没有生成过图片"
                : `共 ${visibleCount} 张 · 点击卡片查看大图`}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            className="h-10 rounded-xl border-stone-200 bg-white/80 px-4 text-stone-700 hover:bg-white"
            onClick={() => void reload()}
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
            <p className="text-sm text-stone-500">从云端拉取你的图片…</p>
          </CardContent>
        </Card>
      ) : null}

      {!isLoading && items.length === 0 ? (
        <Card className="mt-6 rounded-2xl border-white/80 bg-white/90 shadow-sm">
          <CardContent className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
            <div className="rounded-xl bg-stone-100 p-3 text-stone-500">
              <Images className="size-5" />
            </div>
            <div className="space-y-1">
              <p className="text-sm font-medium text-stone-700">这里还很空</p>
              <p className="text-sm text-stone-500">去画图页生成第一张吧</p>
            </div>
            <Button
              variant="outline"
              className="mt-2 h-9 rounded-xl border-stone-200 bg-white px-4 text-stone-700 hover:bg-stone-50"
              onClick={() => window.location.assign("/image")}
            >
              <Sparkles className="size-4" />
              去画图
            </Button>
          </CardContent>
        </Card>
      ) : null}

      {/* Pinterest 风格 masonry：列宽 flex-1 边到边等分容器，不留白。
          - 列数变化时单列宽只变 ~15%，比固定列宽的"侧边留白突变"更柔和
          - 列内"最短列优先"分桶，矮卡紧贴下一张
          - columnCount === 0 表示还没测量，先不渲染避免 SSR/CSR 不一致闪烁
          - 卡片去掉边框/默认 overlay，让图片本身说话；prompt + 时间只在 hover 时浮现
          - 不依赖图片真实高度，只按 aspectRatio 估"列内累计高度"分桶
        */}
      <div
        ref={containerRef}
        className="mt-6 flex gap-3"
        style={{ alignItems: "flex-start" }}
      >
        {columnCount > 0 && (() => {
          const cols = columnCount;
          const buckets: ManagedImage[][] = Array.from({ length: cols }, () => []);
          // 列内累计"高度"近似值：用 1/ratio (= height/width) 做单位列宽下的相对高度
          const heights = new Array(cols).fill(0);
          for (const item of items) {
            const w = item.width && item.width > 0 ? item.width : 1;
            const h = item.height && item.height > 0 ? item.height : 1;
            const relativeH = h / w;
            // 选当前最短列
            let target = 0;
            for (let i = 1; i < cols; i++) {
              if (heights[i] < heights[target]) target = i;
            }
            buckets[target].push(item);
            heights[target] += relativeH;
          }
          return buckets.map((bucket, colIdx) => (
            <div
              key={colIdx}
              className="flex flex-1 flex-col gap-3"
              style={{ minWidth: 0 }}
            >
              {bucket.map((item) => {
                const ratio =
                  item.width && item.height && item.width > 0 && item.height > 0
                    ? item.width / item.height
                    : 1;
                const state = publishStates.get(imageKey(item));
                return (
                  <button
                    key={imageKey(item)}
                    type="button"
                    onClick={() => setFocused(item)}
                    className="group relative w-full cursor-pointer overflow-hidden rounded-2xl bg-stone-100 text-left transition-shadow duration-200 hover:shadow-lg focus-visible:ring-2 focus-visible:ring-stone-900 focus-visible:ring-offset-2 focus-visible:outline-none"
                    style={{ aspectRatio: String(ratio) }}
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={item.url}
                      alt={item.prompt?.slice(0, 30) || item.name}
                      loading="lazy"
                      className="h-full w-full object-cover"
                    />
                    {state === "published" ? (
                      <div className="absolute top-2 left-2 rounded-md bg-emerald-500/95 px-2 py-1 text-[10.5px] font-semibold text-white shadow-sm">
                        已发布
                      </div>
                    ) : null}
                    {/* Pinterest 风格：默认干净纯图，hover 才浮现 prompt + 元信息 */}
                    <div className="pointer-events-none absolute right-0 bottom-0 left-0 bg-gradient-to-t from-black/80 via-black/40 to-transparent p-3 text-white opacity-0 transition-opacity duration-200 group-hover:opacity-100">
                      <p className="line-clamp-2 text-[12.5px] leading-snug">
                        {item.prompt?.trim() || "—"}
                      </p>
                      <div className="mt-1 flex items-center justify-between gap-2 text-[10.5px] text-white/80">
                        <span>{formatRelative(item.created_at)}</span>
                        {item.width && item.height ? (
                          <span className="shrink-0 font-data">
                            {item.width}×{item.height}
                          </span>
                        ) : null}
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
          ));
        })()}
      </div>

      {/* 详情 Dialog */}
      <Dialog open={focused !== null} onOpenChange={(open) => (!open ? setFocused(null) : null)}>
        <DialogContent
          showCloseButton={false}
          className="hide-scrollbar max-h-[92vh] overflow-y-auto rounded-2xl p-0 sm:max-w-[760px]"
        >
          {focusedView ? (
            <div className="flex flex-col">
              {/* 图片 + 右上悬浮操作（关闭/下载/删除）。
                  把次要操作收到角落，底部只留 3 个主 CTA，避免按钮换行。
                  容器底色用 stone-900 兜底；图按容器宽度撑满，高度按比例自然展开，
                  高图由外层 DialogContent 的 max-h-[92vh] + overflow-y-auto 消化滚动。 */}
              <div className="relative bg-stone-900">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={focusedView.url}
                  alt={focusedView.prompt?.slice(0, 30) || focusedView.name}
                  className="block h-auto w-full"
                />
                <div className="absolute top-3 right-3 flex items-center gap-1.5">
                  <button
                    type="button"
                    onClick={() => void handleDownload(focusedView)}
                    aria-label="下载"
                    title="下载"
                    className="grid size-9 cursor-pointer place-items-center rounded-full bg-black/55 text-white backdrop-blur-sm transition hover:bg-black/75"
                  >
                    <Download className="size-4" />
                  </button>
                  <button
                    type="button"
                    onClick={() => setPendingDelete(focusedView)}
                    aria-label="删除"
                    title="删除"
                    className="grid size-9 cursor-pointer place-items-center rounded-full bg-black/55 text-white backdrop-blur-sm transition hover:bg-rose-600"
                  >
                    <Trash2 className="size-4" />
                  </button>
                  <button
                    type="button"
                    onClick={() => setFocused(null)}
                    aria-label="关闭"
                    title="关闭"
                    className="grid size-9 cursor-pointer place-items-center rounded-full bg-black/55 text-white backdrop-blur-sm transition hover:bg-black/75"
                  >
                    <X className="size-4" />
                  </button>
                </div>
              </div>
              <div className="flex flex-col gap-3 p-5">
                <DialogHeader className="gap-1.5 space-y-0">
                  <DialogTitle className="text-base font-semibold">作品详情</DialogTitle>
                  <DialogDescription className="sr-only">单张作品的 prompt 与操作</DialogDescription>
                </DialogHeader>

                {focusedView.prompt ? (
                  <div className="rounded-xl bg-stone-50 p-3 text-[13px] leading-6 text-stone-800">
                    {focusedView.prompt}
                  </div>
                ) : (
                  <div className="rounded-xl border border-dashed border-stone-200 bg-stone-50/70 p-3 text-[12px] leading-6 text-stone-500">
                    此图未保留生成时的 prompt（可能是早期版本生成的）。发布到画廊时会让你手填。
                  </div>
                )}

                <div className="flex flex-wrap items-center gap-2 text-xs text-stone-500">
                  <span>{formatRelative(focusedView.created_at)}</span>
                  {focusedView.width && focusedView.height ? (
                    <span className="font-data">
                      · {focusedView.width}×{focusedView.height}
                    </span>
                  ) : null}
                </div>

                {/* 底部 3 主 CTA 等分宽度，永远不换行；
                    下载/删除已移到图片右上角悬浮按钮。 */}
                <div className="mt-2 grid grid-cols-3 gap-2">
                  <Button
                    onClick={() => handleRedraw(focusedView)}
                    className="h-10 w-full rounded-xl bg-stone-950 px-3 text-white hover:bg-stone-800"
                  >
                    <Sparkles className="size-4" />
                    用此图重画
                  </Button>
                  <Button
                    variant="outline"
                    className="h-10 w-full rounded-xl border-stone-200 bg-white px-3"
                    onClick={() => void handleCopyPrompt(focusedView.prompt || "")}
                    disabled={!focusedView.prompt}
                  >
                    <Copy className="size-4" />
                    复制 prompt
                  </Button>
                  <Button
                    variant="outline"
                    className="h-10 w-full rounded-xl border-stone-200 bg-white px-3"
                    onClick={() => void handlePublish(focusedView)}
                    disabled={focusedPublishState === "publishing" || focusedPublishState === "published"}
                  >
                    {focusedPublishState === "publishing" ? (
                      <LoaderCircle className="size-4 animate-spin" />
                    ) : (
                      <Share2 className="size-4" />
                    )}
                    {focusedPublishState === "published" ? "已发布" : "发布到画廊"}
                  </Button>
                </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      {/* 老数据 / 图生图无 prompt 的图发布前选择性补段描述（可选，留空也能发） */}
      <Dialog
        open={pendingPublish !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingPublish(null);
            setPromptDraft("");
          }
        }}
      >
        <DialogContent showCloseButton={false} className="rounded-2xl p-6">
          <DialogHeader>
            <DialogTitle>给这张图加段 prompt（可选）</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              此图没有保留生成时的 prompt。补一段描述能让其他用户复用提示词，留空也可直接发布。
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={promptDraft}
            onChange={(event) => setPromptDraft(event.target.value)}
            placeholder="比如：一只穿宇航服的猫，蹲在月球表面"
            className="mt-2 min-h-[120px] rounded-xl"
          />
          <DialogFooter className="mt-2">
            <Button
              variant="outline"
              onClick={() => {
                setPendingPublish(null);
                setPromptDraft("");
              }}
              disabled={publishing}
            >
              取消
            </Button>
            <Button
              className="bg-stone-950 text-white hover:bg-stone-800"
              onClick={() => void handleConfirmPendingPublish()}
              disabled={publishing}
            >
              {publishing ? <LoaderCircle className="size-4 animate-spin" /> : null}
              确认发布
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 删除二次确认 */}
      <Dialog
        open={pendingDelete !== null}
        onOpenChange={(open) => (!open ? setPendingDelete(null) : null)}
      >
        <DialogContent showCloseButton={false} className="rounded-2xl p-6">
          <DialogHeader>
            <DialogTitle>删除这张作品？</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              服务器上的图片会一起被删除，画廊里发布过的对应条目也会被移除，已下载到本地的不受影响。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)} disabled={deleting}>
              取消
            </Button>
            <Button
              className="bg-rose-600 text-white hover:bg-rose-700"
              onClick={() => void handleConfirmDelete()}
              disabled={deleting}
            >
              {deleting ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default function WorksPage() {
  const { isCheckingAuth, session } = useAuthGuard();

  if (isCheckingAuth || !session) {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <LoaderCircle className="size-5 animate-spin text-stone-400" />
      </div>
    );
  }

  return <WorksPageContent />;
}
