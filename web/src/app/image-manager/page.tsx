"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CalendarDays, Check, ChevronDown, ChevronLeft, ChevronRight, Copy, Download, ImageIcon, LoaderCircle, Maximize2, Plus, RefreshCw, Search, Share2, Tag, Trash2, User, X } from "lucide-react";
import { toast } from "sonner";

import { DateRangeFilter } from "@/components/date-range-filter";
import { ImageLightbox } from "@/components/image-lightbox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { deleteImageTag, deleteManagedImages, downloadImages, downloadSingleImage, fetchImageOwners, fetchImageTags, fetchManagedImages, getMyPublishedBatch, publishGalleryItem, setImageTags, type ImageOwner, type ManagedImage } from "@/lib/api";
import { useAuthGuard } from "@/lib/use-auth-guard";

const LONG_PRESS_MS = 800;

function formatSize(size: number) {
  return size > 1024 * 1024 ? `${(size / 1024 / 1024).toFixed(2)} MB` : `${Math.ceil(size / 1024)} KB`;
}

function imageKey(item: ManagedImage) {
  return item.rel || item.url;
}

// 用户筛选下拉。max-h 限定 320px，列表本身用 .scrollbar-fancy 走自定义细滚动条，
// 视觉风格与全局 stone 色系保持一致；空状态、未归属、已删用户都显式提示。
// 三类语义置顶：全部用户 / 管理员（__admin__） / 未归属（__unowned__），其余具体用户在分隔线下面可搜索。
function OwnerFilter({
  value,
  owners,
  open,
  onOpenChange,
  onChange,
}: {
  value: string;
  owners: ImageOwner[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (next: string) => void;
}) {
  const [query, setQuery] = useState("");
  // 重置：每次重新打开下拉就清空搜索关键字，避免下次打开还停留在上次的过滤态。
  useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  const adminBucket = owners.find((item) => item.id === "__admin__") ?? null;
  const unownedBucket = owners.find((item) => item.id === "__unowned__") ?? null;
  const realOwners = owners.filter((item) => item.id !== "__admin__" && item.id !== "__unowned__");
  const normalized = query.trim().toLowerCase();
  const filteredOwners = normalized
    ? realOwners.filter(
        (item) =>
          item.name.toLowerCase().includes(normalized) || item.id.toLowerCase().includes(normalized),
      )
    : realOwners;

  const selected = owners.find((item) => item.id === value) ?? null;
  const buttonLabel = !value
    ? "全部用户"
    : value === "__admin__"
      ? "管理员"
      : value === "__unowned__"
        ? "未归属"
        : selected?.name || value;
  const totalCount =
    realOwners.reduce((sum, item) => sum + item.count, 0) +
    (adminBucket?.count ?? 0) +
    (unownedBucket?.count ?? 0);
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="h-10 cursor-pointer rounded-xl border-stone-200 bg-white px-3 text-stone-700 hover:bg-stone-50"
        >
          <User className="size-4 text-stone-500" />
          <span className="max-w-[160px] truncate text-[13px]">{buttonLabel}</span>
          {selected ? (
            <span className="font-data tabular-nums rounded-md bg-stone-100 px-1.5 text-[10px] text-stone-500">
              {selected.count}
            </span>
          ) : null}
          <ChevronDown className="size-3.5 text-stone-400" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-64 overflow-hidden rounded-xl border-stone-200 bg-white p-0 shadow-[0_4px_20px_-4px_rgba(15,23,42,0.18)]"
      >
        <div className="border-b border-stone-100 p-2">
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-stone-400" />
            <input
              autoFocus
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索用户"
              className="h-8 w-full rounded-lg border border-stone-200 bg-white pr-7 pl-7 text-[12.5px] text-stone-700 placeholder:text-stone-400 focus:border-stone-400 focus:outline-none"
            />
            {query ? (
              <button
                type="button"
                onClick={() => setQuery("")}
                className="absolute top-1/2 right-1.5 inline-flex size-5 -translate-y-1/2 cursor-pointer items-center justify-center rounded-md text-stone-400 hover:bg-stone-100 hover:text-stone-600"
                title="清除搜索"
              >
                <X className="size-3" />
              </button>
            ) : null}
          </div>
        </div>
        <div className="scrollbar-fancy max-h-[320px] overflow-y-auto py-1">
          {/* 三个固定导航项：全部 / 管理员 / 未归属。query 非空也保持显示，
              它们是导航类入口，搜索时也希望随时切回。 */}
          <OwnerOption
            label="全部用户"
            hint={`${totalCount} 张`}
            selected={!value}
            onClick={() => onChange("")}
          />
          {adminBucket ? (
            <OwnerOption
              label="管理员"
              hint={`${adminBucket.count} 张`}
              special
              selected={value === "__admin__"}
              onClick={() => onChange("__admin__")}
            />
          ) : null}
          {unownedBucket ? (
            <OwnerOption
              label="未归属"
              hint={`${unownedBucket.count} 张`}
              special
              selected={value === "__unowned__"}
              onClick={() => onChange("__unowned__")}
            />
          ) : null}
          <div className="my-1 h-px bg-stone-100" />
          {realOwners.length === 0 ? (
            <div className="px-3 py-6 text-center text-xs text-stone-400">还没有用户密钥</div>
          ) : filteredOwners.length === 0 ? (
            <div className="px-3 py-6 text-center text-xs text-stone-400">没有匹配的用户</div>
          ) : (
            filteredOwners.map((item) => (
              <OwnerOption
                key={item.id}
                label={item.name}
                hint={`${item.count} 张`}
                deleted={item.deleted}
                selected={value === item.id}
                onClick={() => onChange(item.id)}
              />
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}

function OwnerOption({
  label,
  hint,
  selected,
  deleted,
  special,
  onClick,
}: {
  label: string;
  hint?: string;
  selected: boolean;
  deleted?: boolean;
  special?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex w-full cursor-pointer items-center justify-between gap-2 px-3 py-2 text-left text-sm transition ${
        selected ? "bg-stone-100 text-stone-900" : "text-stone-700 hover:bg-stone-50"
      }`}
    >
      <span className="flex min-w-0 items-center gap-2">
        <span
          className={`flex size-4 shrink-0 items-center justify-center rounded-full ${
            selected ? "bg-stone-900 text-white" : "bg-transparent text-transparent"
          }`}
        >
          <Check className="size-3" />
        </span>
        <span className={`truncate ${special ? "text-stone-500" : ""}`}>{label}</span>
        {deleted ? (
          <Badge variant="secondary" className="rounded-md bg-rose-50 px-1.5 py-0 text-[10px] text-rose-600">
            已删
          </Badge>
        ) : null}
      </span>
      {hint ? (
        <span className="font-data tabular-nums shrink-0 text-[11px] text-stone-400">{hint}</span>
      ) : null}
    </button>
  );
}

// 模块级缓存。组件每次切回 image-manager 都会重新挂载，
// 不缓存的话 items 从 [] 起跳、isLoading=true 让网格高度从 0 撑到 N 行，
// 视觉上是设置页之外最严重的"跳动"页面。
type ImageManagerCache = {
  items: ManagedImage[];
  allTags: string[];
  owners: ImageOwner[];
  startDate: string;
  endDate: string;
  owner: string;
};
let cachedImageManager: ImageManagerCache | null = null;

function useLongPress(onLongPress: () => void, ms = LONG_PRESS_MS) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const activeRef = useRef(false);

  const start = useCallback((e: React.MouseEvent | React.TouchEvent) => {
    activeRef.current = true;
    timerRef.current = setTimeout(() => {
      if (activeRef.current) {
        onLongPress();
      }
    }, ms);
  }, [onLongPress, ms]);

  const stop = useCallback(() => {
    activeRef.current = false;
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  return {
    onMouseDown: start,
    onMouseUp: stop,
    onMouseLeave: stop,
    onTouchStart: start,
    onTouchEnd: stop,
  };
}

function ImageManagerContent() {
  // 命中缓存时直接拿来当初始 state，避免切回时网格塌缩成空再撑回。
  const [items, setItemsState] = useState<ManagedImage[]>(() => cachedImageManager?.items ?? []);
  const [startDate, setStartDate] = useState(() => cachedImageManager?.startDate ?? "");
  const [endDate, setEndDate] = useState(() => cachedImageManager?.endDate ?? "");
  const [owner, setOwner] = useState(() => cachedImageManager?.owner ?? "");
  const [owners, setOwnersState] = useState<ImageOwner[]>(() => cachedImageManager?.owners ?? []);
  const [ownerPickerOpen, setOwnerPickerOpen] = useState(false);
  const [lightboxIndex, setLightboxIndex] = useState(0);
  const [lightboxOpen, setLightboxOpen] = useState(false);
  const [page, setPage] = useState(1);
  const [isLoading, setIsLoading] = useState(() => cachedImageManager === null);
  const [deleteTarget, setDeleteTarget] = useState<ManagedImage | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const [allTags, setAllTagsState] = useState<string[]>(() => cachedImageManager?.allTags ?? []);
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  const [tagEditTarget, setTagEditTarget] = useState<ManagedImage | null>(null);
  const [tagInput, setTagInput] = useState("");
  const [dialogVisible, setDialogVisible] = useState(false);
  const deleteTargetRef = useRef<ManagedImage | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<string[]>([]);
  const [deleteMode, setDeleteMode] = useState<"selected" | "filtered" | null>(null);
  const [isDownloading, setIsDownloading] = useState(false);

  // 发布画廊状态：rel → "publishing" | "published"。
  // admin 视角下未必由当前账号发的，被任何用户发过都标"已发布"，
  // 后端 batch 接口在 admin 请求时自动按 check_any_publisher=True 跨用户查。
  const [publishStates, setPublishStates] = useState<Map<string, "publishing" | "published">>(
    () => new Map(),
  );
  // 发布者展示名：rel → publisher_name。仅用于已发布角标 tooltip 显示"由 xx 发布"，
  // 帮 admin 在管理页快速辨认是谁发的。
  const [publisherNames, setPublisherNames] = useState<Map<string, string>>(() => new Map());
  // 没 prompt 时弹窗手填，复用 works 页同款模式
  const [pendingPublish, setPendingPublish] = useState<ManagedImage | null>(null);
  const [promptDraft, setPromptDraft] = useState("");
  const [publishingDialog, setPublishingDialog] = useState(false);

  // 写 items / allTags 同步刷新缓存，下次切回拿到最新值。
  // 同时支持值与 functional updater 两种形式（旧代码大量用 setItems(prev => ...)）。
  const setItems = useCallback(
    (next: ManagedImage[] | ((prev: ManagedImage[]) => ManagedImage[])) => {
      setItemsState((prev) => {
        const value = typeof next === "function" ? (next as (p: ManagedImage[]) => ManagedImage[])(prev) : next;
        cachedImageManager = {
          items: value,
          allTags: cachedImageManager?.allTags ?? [],
          owners: cachedImageManager?.owners ?? [],
          startDate,
          endDate,
          owner,
        };
        return value;
      });
    },
    [startDate, endDate, owner],
  );
  const setAllTags = useCallback(
    (next: string[] | ((prev: string[]) => string[])) => {
      setAllTagsState((prev) => {
        const value = typeof next === "function" ? (next as (p: string[]) => string[])(prev) : next;
        cachedImageManager = {
          items: cachedImageManager?.items ?? [],
          allTags: value,
          owners: cachedImageManager?.owners ?? [],
          startDate,
          endDate,
          owner,
        };
        return value;
      });
    },
    [startDate, endDate, owner],
  );
  const setOwners = useCallback(
    (next: ImageOwner[]) => {
      // 兜底：永远只把数组写进 state 与缓存。dev 下 Fast Refresh 偶尔会把旧
      // state slot 串过来，给后续 `for..of`/`.find` 等操作炸场，集中拦在写入处。
      const safe = Array.isArray(next) ? next : [];
      setOwnersState(safe);
      cachedImageManager = {
        items: cachedImageManager?.items ?? [],
        allTags: cachedImageManager?.allTags ?? [],
        owners: safe,
        startDate,
        endDate,
        owner,
      };
    },
    [startDate, endDate, owner],
  );

  const filteredItems = selectedTags.length > 0
    ? items.filter((item) => selectedTags.every((t) => (item.tags ?? []).includes(t)))
    : items;

  const lightboxImages = filteredItems.map((item) => ({
    id: item.name,
    src: item.url,
    sizeLabel: formatSize(item.size),
    dimensions: item.width && item.height ? `${item.width} x ${item.height}` : undefined,
  }));
  const pageSize = 12;
  const pageCount = Math.max(1, Math.ceil(filteredItems.length / pageSize));
  const safePage = Math.min(page, pageCount);
  const currentRows = filteredItems.slice((safePage - 1) * pageSize, safePage * pageSize);
  const selectedSet = useMemo(() => new Set(selectedPaths), [selectedPaths]);
  const ownerNameById = useMemo(() => {
    const map = new Map<string, string>();
    // dev Fast Refresh 偶尔会把旧 state slot 串到这里，防一下非数组场景
    if (!Array.isArray(owners)) return map;
    for (const item of owners) {
      if (!item || typeof item !== "object") continue;
      map.set(item.id, item.name || item.id);
    }
    return map;
  }, [owners]);
  const selectedCount = deleteMode === "filtered" ? items.length : selectedPaths.length;
  const currentPageSelected = currentRows.length > 0 && currentRows.every((item) => selectedSet.has(imageKey(item)));
  const allSelected = filteredItems.length > 0 && filteredItems.every((item) => selectedSet.has(imageKey(item)));

  const loadImages = async (silent = false) => {
    if (!silent) setIsLoading(true);
    try {
      const [data, tagsData, ownersData] = await Promise.all([
        fetchManagedImages({ start_date: startDate, end_date: endDate, owner }),
        fetchImageTags(),
        fetchImageOwners(),
      ]);
      setItems(data.items);
      setAllTags(tagsData.tags);
      setOwners(ownersData.items);
      setSelectedPaths((current) => current.filter((path) => data.items.some((item) => imageKey(item) === path)));
      setPage(1);
      // 播种发布状态：admin 视角下后端会跨用户返回所有已发布的 rel。
      // 不阻塞主流程；失败静默，下次 reload 再试。
      const rels = data.items.map((it) => it.rel).filter(Boolean);
      if (rels.length > 0) {
        try {
          const { items: published } = await getMyPublishedBatch(rels);
          setPublishStates((prev) => {
            const next = new Map(prev);
            // 先清掉这一批 rel 的旧状态再写新状态——避免别人撤回了角标还残留
            for (const rel of rels) next.delete(rel);
            for (const [rel, info] of Object.entries(published)) {
              if (info.published) next.set(rel, "published");
            }
            return next;
          });
          setPublisherNames((prev) => {
            const next = new Map(prev);
            for (const rel of rels) next.delete(rel);
            for (const [rel, info] of Object.entries(published)) {
              if (info.publisher_name) next.set(rel, info.publisher_name);
            }
            return next;
          });
        } catch {
          // 静默：拉不到发布状态不阻塞列表
        }
      }
    } catch (error) {
      if (!silent) toast.error(error instanceof Error ? error.message : "加载图片失败");
    } finally {
      if (!silent) setIsLoading(false);
    }
  };

  const closeDialog = useCallback(() => {
    setDialogVisible(false);
    setTimeout(() => setDeleteTarget(null), 200);
  }, []);

  /**
   * 发布到画廊。admin 代发任意用户的图，后端 publish 路由对 admin 跳过 owner 校验。
   *  - 已有 prompt：直接 publish，过敏感词 → 成功 → 给绿对勾视觉
   *  - 没有 prompt：弹个对话框让 admin 手填（可留空），提交时再 publish
   */
  const handlePublish = useCallback(
    async (item: ManagedImage, promptOverride?: string) => {
      const rel = item.rel;
      if (!rel) {
        toast.error("当前图片无法发布");
        return;
      }
      let prompt: string;
      if (promptOverride !== undefined) {
        prompt = promptOverride.trim();
      } else {
        prompt = (item.prompt ?? "").trim();
        if (!prompt) {
          // 卡片自身没 prompt → 弹窗让 admin 决定补不补（留空也能发）
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
    const text = promptDraft.trim();
    setPublishingDialog(true);
    try {
      await handlePublish(pendingPublish, text);
      setPendingPublish(null);
      setPromptDraft("");
    } finally {
      setPublishingDialog(false);
    }
  }, [handlePublish, pendingPublish, promptDraft]);

  const openDeleteDialog = useCallback((item: ManagedImage) => {
    deleteTargetRef.current = item;
    setDeleteTarget(item);
    setDialogVisible(true);
  }, []);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setIsDeleting(true);
    try {
      await deleteManagedImages({ paths: [deleteTarget.rel] });
      setItems((prev) => prev.filter((item) => item.rel !== deleteTarget.rel));
      setSelectedPaths((prev) => prev.filter((p) => p !== imageKey(deleteTarget)));
      toast.success("图片已删除");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除失败");
    } finally {
      setIsDeleting(false);
      closeDialog();
    }
  };

  const handleSetTags = async (item: ManagedImage, tags: string[]) => {
    try {
      const result = await setImageTags(item.rel, tags);
      setItems((prev) => prev.map((i) => i.rel === item.rel ? { ...i, tags: result.tags } : i));
      const tagsData = await fetchImageTags();
      setAllTags(tagsData.tags);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "设置标签失败");
    }
  };

  const handleAddTag = (item: ManagedImage) => {
    const tag = tagInput.trim();
    if (!tag) return;
    const current = item.tags ?? [];
    if (current.includes(tag)) {
      toast.error("标签已存在");
      return;
    }
    void handleSetTags(item, [...current, tag]);
    setTagInput("");
  };

  const handleRemoveTag = (item: ManagedImage, tag: string) => {
    void handleSetTags(item, (item.tags ?? []).filter((t) => t !== tag));
  };

  const toggleFilterTag = (tag: string) => {
    setSelectedTags((prev) => prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag]);
    setPage(1);
  };

  const [pressingTag, setPressingTag] = useState<string | null>(null);
  const pressTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [tagDeleteTarget, setTagDeleteTarget] = useState<string | null>(null);

  const handleDeleteTag = async (tag: string) => {
    try {
      const result = await deleteImageTag(tag);
      setAllTags((prev) => prev.filter((t) => t !== tag));
      setSelectedTags((prev) => prev.filter((t) => t !== tag));
      setItems((prev) => prev.map((item) => ({
        ...item,
        tags: (item.tags ?? []).filter((t) => t !== tag),
      })));
      toast.success(`标签"${tag}"已删除，影响 ${result.removed_from} 张图片`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除标签失败");
    }
  };

  const startTagPress = useCallback((tag: string) => {
    setPressingTag(tag);
    pressTimerRef.current = setTimeout(() => {
      setPressingTag(null);
      setTagDeleteTarget(tag);
    }, LONG_PRESS_MS);
  }, []);

  const stopTagPress = useCallback(() => {
    setPressingTag(null);
    if (pressTimerRef.current) {
      clearTimeout(pressTimerRef.current);
      pressTimerRef.current = null;
    }
  }, []);

  const clearFilters = () => {
    setStartDate("");
    setEndDate("");
    setOwner("");
    setSelectedTags([]);
  };

  const togglePaths = (paths: string[], checked: boolean) => {
    setSelectedPaths((current) => checked ? Array.from(new Set([...current, ...paths])) : current.filter((path) => !paths.includes(path)));
  };

  const confirmDelete = async () => {
    if (!deleteMode || selectedCount === 0) return;
    setIsDeleting(true);
    try {
      const data = await deleteManagedImages(
        deleteMode === "filtered"
          ? { start_date: startDate, end_date: endDate, owner, all_matching: true }
          : { paths: selectedPaths },
      );
      toast.success(`已删除 ${data.removed} 张图片`);
      setDeleteMode(null);
      setSelectedPaths([]);
      await loadImages();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除图片失败");
    } finally {
      setIsDeleting(false);
    }
  };

  const handleBatchDownload = async () => {
    const paths = deleteMode === "filtered" ? items.map((item) => item.rel) : selectedPaths;
    if (paths.length === 0) return;
    setIsDownloading(true);
    try {
      await downloadImages(paths);
      toast.success(`已下载 ${paths.length} 张图片`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "下载失败");
    } finally {
      setIsDownloading(false);
    }
  };

  const handleSingleDownload = async (item: ManagedImage) => {
    await downloadSingleImage(item.rel);
  };

  // 首次挂载且缓存命中（filter 与缓存一致）→ 静默刷新；
  // 之后改 filter 触发的 effect 都正常 spinner。
  const isFirstRunRef = useRef(true);
  useEffect(() => {
    const isFirst = isFirstRunRef.current;
    isFirstRunRef.current = false;
    const cacheMatches =
      !!cachedImageManager &&
      cachedImageManager.startDate === startDate &&
      cachedImageManager.endDate === endDate &&
      cachedImageManager.owner === owner;
    void loadImages(isFirst && cacheMatches);
  }, [startDate, endDate, owner]);

  return (
    <section className="mt-4 space-y-5 sm:mt-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-1">
          <div className="text-xs font-semibold tracking-[0.18em] text-stone-500 uppercase">Images</div>
          <h1 className="text-2xl font-semibold tracking-tight">图片管理</h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <DateRangeFilter startDate={startDate} endDate={endDate} onChange={(start, end) => { setStartDate(start); setEndDate(end); }} />
          <OwnerFilter
            value={owner}
            owners={owners}
            open={ownerPickerOpen}
            onOpenChange={setOwnerPickerOpen}
            onChange={(next) => {
              setOwner(next);
              setOwnerPickerOpen(false);
            }}
          />
          <Button variant="outline" onClick={clearFilters} className="h-10 rounded-xl border-stone-200 bg-white px-4 text-stone-700">
            清除筛选条件
          </Button>
          <Button onClick={() => void loadImages()} disabled={isLoading} className="h-10 rounded-xl bg-stone-950 px-4 text-white hover:bg-stone-800">
            {isLoading ? <LoaderCircle className="size-4 animate-spin" /> : <Search className="size-4" />}
            查询
          </Button>
          <Button variant="outline" onClick={() => setDeleteMode("filtered")} disabled={isDeleting || items.length === 0 || (!startDate && !endDate && !owner)} className="h-10 rounded-xl border-rose-200 bg-white px-4 text-rose-600 hover:bg-rose-50">
            <Trash2 className="size-4" />
            删除匹配结果
          </Button>
        </div>
      </div>

      {allTags.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-medium text-stone-500">
            <Tag className="mr-1 inline size-3.5" />
            标签筛选：
          </span>
          {allTags.map((tag) => {
            const isPressing = pressingTag === tag;
            return (
              <span
                key={tag}
                className="relative inline-flex items-center"
                onMouseDown={() => startTagPress(tag)}
                onMouseUp={stopTagPress}
                onMouseLeave={stopTagPress}
                onTouchStart={() => startTagPress(tag)}
                onTouchEnd={stopTagPress}
              >
                <button
                  type="button"
                  onClick={() => toggleFilterTag(tag)}
                >
                  <Badge
                    variant={selectedTags.includes(tag) ? "default" : "outline"}
                    className={`cursor-pointer rounded-md transition-all hover:opacity-80 ${isPressing ? "ring-2 ring-red-400 ring-offset-1" : ""}`}
                  >
                    {tag}
                  </Badge>
                </button>
                {isPressing ? (
                  <span className="pointer-events-none absolute inset-0 overflow-hidden rounded-md">
                    <span className="absolute inset-0 animate-[grow_800ms_linear_forwards] rounded-md bg-red-400/20" />
                  </span>
                ) : null}
              </span>
            );
          })}
          {selectedTags.length > 0 ? (
            <button type="button" onClick={() => setSelectedTags([])}>
              <Badge variant="secondary" className="cursor-pointer rounded-md">
                <X className="mr-0.5 size-3" />
                清除
              </Badge>
            </button>
          ) : null}
        </div>
      ) : null}

      <Card className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
        <CardContent className="p-0">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-100 px-5 py-4">
            <div className="flex flex-wrap items-center gap-3 text-sm text-stone-600">
              <ImageIcon className="size-4" />
              共 {filteredItems.length} 张
              {selectedTags.length > 0 ? <span className="text-stone-400">（筛选自 {items.length} 张）</span> : null}
              <label className="flex items-center gap-2">
                <Checkbox checked={currentPageSelected} onCheckedChange={(checked) => togglePaths(currentRows.map(imageKey), Boolean(checked))} />
                本页全选
              </label>
              <label className="flex items-center gap-2">
                <Checkbox checked={allSelected} onCheckedChange={(checked) => togglePaths(filteredItems.map(imageKey), Boolean(checked))} />
                全选结果
              </label>
              {selectedPaths.length > 0 ? <span>已选 {selectedPaths.length} 张</span> : null}
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" className="h-8 rounded-lg px-3 text-stone-500" onClick={() => void loadImages()} disabled={isLoading}>
                <RefreshCw className={`size-4 ${isLoading ? "animate-spin" : ""}`} />
                刷新
              </Button>
              <button type="button" className="text-sm text-stone-500 hover:text-stone-900 disabled:text-stone-300" onClick={() => setSelectedPaths([])} disabled={selectedPaths.length === 0 || isDeleting}>
                取消选择
              </button>
              <Button variant="outline" className="h-8 rounded-lg border-stone-200 bg-white px-3 text-stone-600 hover:bg-stone-50" onClick={() => void handleBatchDownload()} disabled={selectedPaths.length === 0 || isDownloading || isDeleting}>
                {isDownloading ? <LoaderCircle className="size-4 animate-spin" /> : <Download className="size-4" />}
                下载所选
              </Button>
              <Button variant="outline" className="h-8 rounded-lg border-rose-200 bg-white px-3 text-rose-600 hover:bg-rose-50" onClick={() => setDeleteMode("selected")} disabled={selectedPaths.length === 0 || isDeleting}>
                <Trash2 className="size-4" />
                删除所选
              </Button>
            </div>
          </div>
          <div className="grid gap-0 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
            {currentRows.map((item) => {
              const imageIndex = filteredItems.findIndex((row) => row.url === item.url);
              const publishState = publishStates.get(item.rel);
              const publishedBy = publisherNames.get(item.rel);
              return (
              <div key={item.rel} className="group border-r border-b border-stone-100 p-4 transition hover:bg-stone-50">
                <div className="relative">
                  <button
                    type="button"
                    className="relative block aspect-square w-full cursor-zoom-in overflow-hidden rounded-lg bg-stone-100 text-left"
                    onClick={() => {
                      setLightboxIndex(imageIndex);
                      setLightboxOpen(true);
                    }}
                  >
                    <img
                      src={item.thumbnail_url || item.url}
                      alt={item.name}
                      className="h-full w-full object-cover transition group-hover:scale-[1.02]"
                      onError={(event) => {
                        if (event.currentTarget.src !== item.url) {
                          event.currentTarget.src = item.url;
                        }
                      }}
                    />
                    <span className="absolute right-2 bottom-2 rounded-full bg-black/50 p-2 text-white opacity-100 transition sm:opacity-0 sm:group-hover:opacity-100">
                      <Maximize2 className="size-4" />
                    </span>
                  </button>
                  {/* 左上"已发布"角标：只要这张图被任何人发布过画廊就显示。
                      tooltip 注明发布者名（admin 视角下后端会附带 publisher_name），
                      帮 admin 快速辨认是谁发的；普通登录态进不来这页。 */}
                  {publishState === "published" ? (
                    <div
                      className="absolute top-2 left-2 z-10 rounded-md bg-emerald-500/95 px-2 py-1 text-[10.5px] font-semibold text-white shadow-sm"
                      title={publishedBy ? `由 ${publishedBy} 发布` : "已发布到画廊"}
                    >
                      已发布
                    </div>
                  ) : null}
                  <button
                    type="button"
                    className="absolute top-2 right-2 z-10 inline-flex size-7 items-center justify-center rounded-full bg-black/50 text-white opacity-100 transition hover:bg-red-600 sm:opacity-0 sm:group-hover:opacity-100"
                    title="删除图片"
                    onClick={(e) => {
                      e.stopPropagation();
                      openDeleteDialog(item);
                    }}
                  >
                    <Trash2 className="size-3.5" />
                  </button>
                </div>
                <div className="mt-3 space-y-2 text-xs text-stone-500">
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex min-w-0 items-center gap-2">
                      <span className="flex items-center gap-1 font-medium text-stone-700">
                        <CalendarDays className="size-3.5" />
                        {item.created_at}
                      </span>
                    </div>
                    <div className="flex items-center gap-1">
                      {/* 发布到画廊：已发布时变 emerald 实色不可再点；publishing 转圈。
                          stopPropagation 防止冒泡触发卡片大图。 */}
                      <Button
                        variant="ghost"
                        size="icon"
                        className={`size-8 rounded-lg ${
                          publishState === "published"
                            ? "text-emerald-600 hover:bg-emerald-50 hover:text-emerald-700"
                            : "text-stone-400 hover:bg-stone-100 hover:text-stone-700"
                        }`}
                        onClick={(e) => {
                          e.stopPropagation();
                          void handlePublish(item);
                        }}
                        disabled={publishState === "publishing" || publishState === "published"}
                        title={
                          publishState === "published"
                            ? publishedBy
                              ? `已发布到画廊（${publishedBy}）`
                              : "已发布到画廊"
                            : "发布到画廊"
                        }
                      >
                        {publishState === "publishing" ? (
                          <LoaderCircle className="size-4 animate-spin" />
                        ) : (
                          <Share2 className="size-4" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8 rounded-lg text-stone-400 hover:bg-stone-100 hover:text-stone-700"
                        onClick={() => void handleSingleDownload(item)}
                        title="下载图片"
                      >
                        <Download className="size-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8 rounded-lg text-stone-400 hover:bg-stone-100 hover:text-stone-700"
                        onClick={() => {
                          void navigator.clipboard.writeText(item.url);
                          toast.success("图片地址已复制");
                        }}
                      >
                        <Copy className="size-4" />
                      </Button>
                      <Checkbox checked={selectedSet.has(imageKey(item))} onCheckedChange={(checked) => togglePaths([imageKey(item)], Boolean(checked))} />
                    </div>
                  </div>
                  <div className="flex items-center justify-between gap-2">
                    <span>{formatSize(item.size)}</span>
                    <span>{item.width && item.height ? `${item.width} x ${item.height}` : "-"}</span>
                  </div>
                  <div className="flex flex-wrap items-center gap-1">
                    {item.owner_id ? (() => {
                      // 三类来源不同的展示规则：
                      // - admin（包括旧 auth_key 的固定 id "admin"）：统一显示"管理员"，不暴露具体密钥名
                      // - 普通用户：显示用户名（ownerNameById 已涵盖），找不到时兜底显示截断 id
                      // - 真孤儿（owner_id 为空）：上面的条件已经过滤掉了
                      const isAdmin = item.is_admin_owner || item.owner_id === "admin";
                      const display = isAdmin ? "管理员" : (ownerNameById.get(item.owner_id) || item.owner_id);
                      return (
                        <Badge
                          variant="outline"
                          className="gap-0.5 rounded-md border-stone-200 bg-stone-50 px-1.5 py-0 text-[10px] font-medium text-stone-600"
                          title={`生成者：${display}`}
                        >
                          <User className="size-2.5 text-stone-400" />
                          <span className="max-w-[88px] truncate">{display}</span>
                        </Badge>
                      );
                    })() : null}
                    {(item.tags ?? []).map((tag) => (
                      <Badge key={tag} variant="secondary" className="gap-0.5 rounded-md py-0 pr-0.5 text-[10px]">
                        {tag}
                        <button
                          type="button"
                          className="inline-flex size-3.5 items-center justify-center rounded-full hover:bg-stone-300"
                          onClick={() => handleRemoveTag(item, tag)}
                        >
                          <X className="size-2.5" />
                        </button>
                      </Badge>
                    ))}
                    <Popover open={tagEditTarget?.rel === item.rel} onOpenChange={(open) => { setTagEditTarget(open ? item : null); setTagInput(""); }}>
                      <PopoverTrigger asChild>
                        <button
                          type="button"
                          className="inline-flex size-5 items-center justify-center rounded-full border border-dashed border-stone-300 text-stone-400 hover:border-stone-500 hover:text-stone-600"
                          title="添加标签"
                        >
                          <Plus className="size-3" />
                        </button>
                      </PopoverTrigger>
                      <PopoverContent align="start" className="w-56 p-2">
                        <div className="space-y-2">
                          <div className="text-xs font-medium text-stone-500">添加标签</div>
                          <div className="flex gap-1">
                            <Input
                              value={tagInput}
                              onChange={(e) => setTagInput(e.target.value)}
                              placeholder="输入标签名"
                              className="h-8 text-xs"
                              onKeyDown={(e) => {
                                if (e.key === "Enter") {
                                  e.preventDefault();
                                  handleAddTag(item);
                                }
                              }}
                            />
                            <Button
                              size="icon"
                              variant="outline"
                              className="size-8 shrink-0"
                              onClick={() => handleAddTag(item)}
                            >
                              <Plus className="size-3.5" />
                            </Button>
                          </div>
                          {allTags.filter((t) => !(item.tags ?? []).includes(t)).length > 0 ? (
                            <div className="flex flex-wrap gap-1 border-t border-stone-100 pt-2">
                              {allTags.filter((t) => !(item.tags ?? []).includes(t)).map((tag) => (
                                <button
                                  key={tag}
                                  type="button"
                                  onClick={() => {
                                    void handleSetTags(item, [...(item.tags ?? []), tag]);
                                    setTagEditTarget(null);
                                  }}
                                >
                                  <Badge variant="outline" className="cursor-pointer rounded-md text-[10px] hover:bg-stone-100">
                                    {tag}
                                  </Badge>
                                </button>
                              ))}
                            </div>
                          ) : null}
                        </div>
                      </PopoverContent>
                    </Popover>
                  </div>
                </div>
              </div>
            )})}
          </div>
          <div className="flex items-center justify-end gap-2 border-t border-stone-100 px-4 py-3 text-sm text-stone-500">
            <span>第 {safePage} / {pageCount} 页，共 {filteredItems.length} 张</span>
            <Button variant="outline" size="icon" className="size-9 rounded-lg border-stone-200 bg-white" disabled={safePage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>
              <ChevronLeft className="size-4" />
            </Button>
            <Button variant="outline" size="icon" className="size-9 rounded-lg border-stone-200 bg-white" disabled={safePage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}>
              <ChevronRight className="size-4" />
            </Button>
          </div>
          {!isLoading && filteredItems.length === 0 ? <div className="px-6 py-14 text-center text-sm text-stone-500">没有找到图片</div> : null}
        </CardContent>
      </Card>

      <Dialog open={dialogVisible} onOpenChange={(open) => { if (!open) closeDialog(); }}>
        <DialogContent className="max-w-sm overflow-hidden rounded-2xl">
          <DialogHeader>
            <DialogTitle className="pr-8">确认删除</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-stone-600">
            确定要删除这张图片吗？此操作不可恢复。
          </p>
          {deleteTarget ? (
            <div className="flex items-center gap-3 overflow-hidden rounded-xl border border-stone-200 bg-stone-50 p-3">
              <img
                src={deleteTarget.thumbnail_url || deleteTarget.url}
                alt=""
                className="size-16 shrink-0 rounded-lg object-cover"
                onError={(e) => { if (e.currentTarget.src !== deleteTarget.url) e.currentTarget.src = deleteTarget.url; }}
              />
              <div className="min-w-0 overflow-hidden text-xs text-stone-500">
                <div className="truncate font-medium text-stone-700">{deleteTarget.name}</div>
                <div className="truncate">{deleteTarget.created_at}</div>
                <div>{formatSize(deleteTarget.size)}</div>
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={closeDialog} className="rounded-xl">
              取消
            </Button>
            <Button variant="destructive" onClick={() => void handleDelete()} disabled={isDeleting} className="rounded-xl">
              {isDeleting ? <LoaderCircle className="mr-1 size-4 animate-spin" /> : <Trash2 className="mr-1 size-4" />}
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ImageLightbox
        images={lightboxImages}
        currentIndex={lightboxIndex}
        open={lightboxOpen}
        onOpenChange={setLightboxOpen}
        onIndexChange={setLightboxIndex}
      />
      <Dialog open={Boolean(deleteMode)} onOpenChange={(open) => (!open ? setDeleteMode(null) : null)}>
        <DialogContent showCloseButton={false} className="rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>{deleteMode === "filtered" ? "删除匹配结果" : "删除所选图片"}</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-stone-600">
            确认删除 {selectedCount} 张图片吗？删除后无法恢复。
          </p>
          <DialogFooter>
            <Button variant="outline" className="rounded-xl" onClick={() => setDeleteMode(null)} disabled={isDeleting}>
              取消
            </Button>
            <Button className="rounded-xl bg-rose-600 text-white hover:bg-rose-700" onClick={() => void confirmDelete()} disabled={isDeleting || selectedCount === 0}>
              {isDeleting ? <LoaderCircle className="size-4 animate-spin" /> : null}
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(tagDeleteTarget)} onOpenChange={(open) => { if (!open) setTagDeleteTarget(null); }}>
        <DialogContent className="max-w-sm rounded-2xl">
          <DialogHeader>
            <DialogTitle>删除标签</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-stone-600">
            确定要删除标签 <span className="font-semibold">"{tagDeleteTarget}"</span> 吗？将从所有图片中移除该标签。
          </p>
          <DialogFooter>
            <Button variant="outline" className="rounded-xl" onClick={() => setTagDeleteTarget(null)}>
              取消
            </Button>
            <Button
              variant="destructive"
              className="rounded-xl"
              onClick={() => {
                if (tagDeleteTarget) void handleDeleteTag(tagDeleteTarget);
                setTagDeleteTarget(null);
              }}
            >
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 发布到画廊：当图片自身没保留 prompt（早期生成 / 图生图无文本）时
          弹这个对话框让 admin 决定补不补。允许留空——后端 publish 已支持空 prompt。 */}
      <Dialog
        open={Boolean(pendingPublish)}
        onOpenChange={(open) => {
          if (!open) {
            setPendingPublish(null);
            setPromptDraft("");
          }
        }}
      >
        <DialogContent className="max-w-md rounded-2xl">
          <DialogHeader>
            <DialogTitle>发布到画廊</DialogTitle>
            <DialogDescription className="text-stone-500">
              此图未保留生成时的 prompt，可以补一段描述，或直接留空发布。
            </DialogDescription>
          </DialogHeader>
          {pendingPublish ? (
            <div className="flex items-center gap-3 overflow-hidden rounded-xl border border-stone-200 bg-stone-50 p-3">
              <img
                src={pendingPublish.thumbnail_url || pendingPublish.url}
                alt=""
                className="size-16 shrink-0 rounded-lg object-cover"
                onError={(e) => { if (e.currentTarget.src !== pendingPublish.url) e.currentTarget.src = pendingPublish.url; }}
              />
              <div className="min-w-0 overflow-hidden text-xs text-stone-500">
                <div className="truncate font-medium text-stone-700">{pendingPublish.name}</div>
                <div className="truncate">{pendingPublish.created_at}</div>
              </div>
            </div>
          ) : null}
          <Input
            value={promptDraft}
            onChange={(e) => setPromptDraft(e.target.value)}
            placeholder="可选：为这张图补一段 prompt"
            className="h-10 rounded-xl"
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey && !publishingDialog) {
                e.preventDefault();
                void handleConfirmPendingPublish();
              }
            }}
          />
          <DialogFooter>
            <Button
              variant="outline"
              className="rounded-xl"
              onClick={() => {
                setPendingPublish(null);
                setPromptDraft("");
              }}
              disabled={publishingDialog}
            >
              取消
            </Button>
            <Button
              className="rounded-xl bg-stone-950 text-white hover:bg-stone-800"
              onClick={() => void handleConfirmPendingPublish()}
              disabled={publishingDialog}
            >
              {publishingDialog ? <LoaderCircle className="size-4 animate-spin" /> : <Share2 className="size-4" />}
              发布
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

export default function ImageManagerPage() {
  const { isCheckingAuth, session } = useAuthGuard(["admin"]);
  if (isCheckingAuth || !session || session.role !== "admin") {
    return <div className="flex min-h-[40vh] items-center justify-center"><LoaderCircle className="size-5 animate-spin text-stone-400" /></div>;
  }
  return <ImageManagerContent />;
}
