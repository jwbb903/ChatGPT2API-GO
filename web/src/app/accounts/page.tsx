"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { ComponentProps } from "react";
import {
  Ban,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleAlert,
  CircleOff,
  Copy,
  Download,
  LayoutGrid,
  LoaderCircle,
  Pencil,
  RefreshCw,
  Rows3,
  Search,
  Trash2,
  UserRound,
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
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  deleteAccounts,
  deleteAccountsWithOptions,
  fetchAccounts,
  refreshAccounts,
  updateAccount,
  type Account,
  type AccountStatus,
} from "@/lib/api";
import { useAuthGuard } from "@/lib/use-auth-guard";
import { cn } from "@/lib/utils";

import { AccountImportDialog } from "./components/account-import-dialog";

const accountStatusOptions: { label: string; value: AccountStatus | "all" }[] = [
  { label: "全部状态", value: "all" },
  { label: "正常", value: "正常" },
  { label: "限流", value: "限流" },
  { label: "异常", value: "异常" },
  { label: "禁用", value: "禁用" },
];

const statusMeta: Record<
  AccountStatus,
  {
    icon: typeof CheckCircle2;
    badge: ComponentProps<typeof Badge>["variant"];
  }
> = {
  正常: { icon: CheckCircle2, badge: "success" },
  限流: { icon: CircleAlert, badge: "warning" },
  异常: { icon: CircleOff, badge: "danger" },
  禁用: { icon: Ban, badge: "secondary" },
};

const metricCards = [
  { key: "total", label: "账户总数", color: "text-stone-900", icon: UserRound },
  { key: "active", label: "正常账户", color: "text-emerald-600", icon: CheckCircle2 },
  { key: "limited", label: "限流账户", color: "text-orange-500", icon: CircleAlert },
  { key: "abnormal", label: "异常账户", color: "text-rose-500", icon: CircleOff },
  { key: "disabled", label: "禁用账户", color: "text-stone-500", icon: Ban },
  { key: "quota", label: "剩余额度", color: "text-blue-500", icon: RefreshCw },
] as const;

function isUnlimitedImageQuotaAccount(account: Account) {
  return account.type === "pro" || account.type === "prolite";
}

function imageQuotaUnknown(account: Account) {
  return Boolean(account.image_quota_unknown);
}

function formatCompact(value: number) {
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}k`;
  }
  return String(value);
}

function formatQuota(account: Account) {
  if (isUnlimitedImageQuotaAccount(account)) {
    return "∞";
  }
  if (imageQuotaUnknown(account)) {
    return "未知";
  }
  return String(Math.max(0, account.quota));
}

function formatRestoreAt(value?: string | null) {
  if (!value) {
    return { absolute: "—", relative: "" };
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return { absolute: value, relative: "" };
  }

  const diffMs = Math.max(0, date.getTime() - Date.now());
  const totalHours = Math.ceil(diffMs / (1000 * 60 * 60));
  const days = Math.floor(totalHours / 24);
  const hours = totalHours % 24;
  const relative = diffMs > 0 ? `剩余 ${days}d ${hours}h` : "已到恢复时间";

  const pad = (num: number) => String(num).padStart(2, "0");
  const absolute = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
    date.getHours(),
  )}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;

  return { absolute, relative };
}

function formatDateTime(value?: string | null) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (num: number) => String(num).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function formatQuotaSummary(accounts: Account[]) {
  const availableAccounts = accounts.filter((account) => account.status === "正常");
  if (availableAccounts.some(isUnlimitedImageQuotaAccount)) {
    return "∞";
  }
  if (availableAccounts.some(imageQuotaUnknown)) {
    return "未知";
  }
  return formatCompact(availableAccounts.reduce((sum, account) => sum + Math.max(0, account.quota), 0));
}

function maskToken(token?: string) {
  if (!token) return "—";
  if (token.length <= 18) return token;
  return `${token.slice(0, 16)}...${token.slice(-8)}`;
}

function downloadTokens(accounts: Account[]) {
  const content = `${accounts.map((account) => account.access_token).join("\n")}\n`;
  const blob = new Blob([content], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `accounts-${Date.now()}.txt`;
  link.click();
  URL.revokeObjectURL(url);
}

function displayAccountType(account: Account) {
  return account.type || "Free";
}

function displayAccountSource(account: Account) {
  return String(account.source_type || "web").toLowerCase() === "codex" ? "Codex" : "Web";
}

const TYPE_BADGE_CLASS: Record<string, string> = {
  Free: "border-stone-200 bg-stone-50 text-stone-700",
  Plus: "border-blue-200 bg-blue-50 text-blue-700",
  pro: "border-violet-200 bg-violet-50 text-violet-700",
  prolite: "border-emerald-200 bg-emerald-50 text-emerald-700",
};

const SOURCE_BADGE_CLASS: Record<"Web" | "Codex", string> = {
  Web: "border-stone-200 bg-stone-50 text-stone-600",
  Codex: "border-cyan-200 bg-cyan-50 text-cyan-700",
};

const STATUS_BADGE_CLASS: Record<AccountStatus, string> = {
  正常: "border-emerald-200 bg-emerald-50 text-emerald-700",
  限流: "border-amber-200 bg-amber-50 text-amber-700",
  异常: "border-rose-200 bg-rose-50 text-rose-700",
  禁用: "border-stone-200 bg-stone-100 text-stone-600",
};

function quotaUpperBound(account: Account) {
  // 优先用注册时拿到的总额度（后端 _normalize_account 维护）；
  // 兜底：按类型估（Plus 50 / 其他 25），主要用于历史数据还没来得及补 initial_quota 的场景。
  const initial = Math.max(0, Math.floor(account.initial_quota ?? 0));
  if (initial > 0) return initial;
  return displayAccountType(account) === "Plus" ? 50 : 25;
}

function PillProgress({
  percent,
  segments = 15,
}: {
  percent: number;
  segments?: number;
}) {
  const clamped = Math.max(0, Math.min(100, percent));
  const filled = Math.round((clamped / 100) * segments);
  return (
    <div className="flex gap-[3px]" aria-label={`${clamped}%`}>
      {Array.from({ length: segments }).map((_, i) => (
        <span
          key={i}
          className={cn(
            "h-2 flex-1 rounded-full transition-colors",
            i < filled ? "bg-emerald-500" : "bg-amber-400/75",
          )}
        />
      ))}
    </div>
  );
}

function HealthPills({
  success,
  fail,
  segments = 14,
}: {
  success: number;
  fail: number;
  segments?: number;
}) {
  const total = success + fail;
  const successCount = total > 0 ? Math.round((success / total) * segments) : 0;
  const failCount = total > 0 ? segments - successCount : 0;
  const idleCount = total > 0 ? 0 : segments;
  return (
    <div className="flex gap-[3px]" aria-label={`成功 ${success} / 失败 ${fail}`}>
      {Array.from({ length: successCount }).map((_, i) => (
        <span key={`s${i}`} className="h-1.5 flex-1 rounded-full bg-emerald-500/85" />
      ))}
      {Array.from({ length: failCount }).map((_, i) => (
        <span key={`f${i}`} className="h-1.5 flex-1 rounded-full bg-rose-500/80" />
      ))}
      {Array.from({ length: idleCount }).map((_, i) => (
        <span key={`i${i}`} className="h-1.5 flex-1 rounded-full bg-secondary" />
      ))}
    </div>
  );
}

function AccountCard({
  account,
  selected,
  onSelectChange,
  onEdit,
  onRefresh,
  onDelete,
  onCopyToken,
  isRefreshing,
  isDeleting,
  isUpdating,
}: {
  account: Account;
  selected: boolean;
  onSelectChange: (v: boolean) => void;
  onEdit: () => void;
  onRefresh: () => void;
  onDelete: () => void;
  onCopyToken: () => void;
  isRefreshing: boolean;
  isDeleting: boolean;
  isUpdating: boolean;
}) {
  const typeName = displayAccountType(account);
  const sourceName = displayAccountSource(account);
  const typeAvatar = (typeName[0] || "?").toUpperCase();
  const isUnlimited = isUnlimitedImageQuotaAccount(account);
  const isUnknown = imageQuotaUnknown(account);
  const maxQuota = quotaUpperBound(account);
  const remainingPct = isUnlimited
    ? 100
    : isUnknown
      ? 0
      : Math.max(0, Math.min(100, Math.round((account.quota / maxQuota) * 100)));
  const restore = formatRestoreAt(account.restore_at);

  return (
    <div
      className={cn(
        "group flex flex-col rounded-2xl border border-border bg-card p-4 shadow-sm transition hover:border-primary/40 hover:shadow-md",
        selected && "border-primary/60 ring-1 ring-primary/20",
      )}
    >
      <div className="flex items-center gap-3">
        <Checkbox
          checked={selected}
          onCheckedChange={(c) => onSelectChange(Boolean(c))}
        />
        <div className="grid size-9 shrink-0 place-items-center rounded-full bg-gradient-to-br from-primary/25 to-primary/5 text-sm font-semibold text-primary">
          {typeAvatar}
        </div>
        <div className="flex flex-1 flex-wrap items-center gap-1.5">
          <Badge
            variant="outline"
            className={cn("rounded-md font-medium", TYPE_BADGE_CLASS[typeName] ?? TYPE_BADGE_CLASS.Free)}
          >
            {typeName}
          </Badge>
          <Badge
            variant="outline"
            className={cn("rounded-md font-medium", SOURCE_BADGE_CLASS[sourceName])}
          >
            {sourceName}
          </Badge>
          <Badge
            variant="outline"
            className={cn("rounded-md font-medium", STATUS_BADGE_CLASS[account.status])}
          >
            {account.status}
          </Badge>
        </div>
      </div>

      <div className="mt-3 truncate text-[15px] font-semibold text-foreground" title={account.email ?? ""}>
        {account.email ?? "—"}
      </div>

      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
        <span className="inline-flex items-center gap-1">
          <span>Token</span>
          <span className="font-data text-foreground/80">{maskToken(account.access_token)}</span>
        </span>
        {restore.absolute !== "—" ? (
          <span className="inline-flex items-center gap-1">
            <span>恢复</span>
            <span className="font-data text-foreground/80">{restore.absolute}</span>
          </span>
        ) : null}
      </div>

      <div className="mt-2.5 flex items-center gap-3 text-xs">
        <span className="inline-flex items-center gap-1">
          <span className="size-1.5 rounded-full bg-emerald-500" />
          <span className="text-muted-foreground">成功</span>
          <span className="font-data tabular-nums font-semibold text-emerald-600">{account.success}</span>
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="size-1.5 rounded-full bg-rose-500" />
          <span className="text-muted-foreground">失败</span>
          <span className="font-data tabular-nums font-semibold text-rose-500">{account.fail}</span>
        </span>
      </div>

      <div className="mt-3 space-y-1.5">
        <div className="font-data text-[10px] font-semibold tracking-[0.18em] uppercase text-muted-foreground">
          健康状态
        </div>
        <HealthPills success={account.success} fail={account.fail} />
      </div>

      <div className="mt-3 space-y-1.5">
        <div className="flex items-baseline justify-between">
          <span className="font-data text-[10px] font-semibold tracking-[0.18em] uppercase text-muted-foreground">
            剩余额度
          </span>
          <span className="font-data tabular-nums text-xs">
            {isUnlimited ? (
              <span className="font-semibold text-emerald-600">∞</span>
            ) : isUnknown ? (
              <span className="text-muted-foreground">未知</span>
            ) : (
              <>
                <span className="font-semibold text-foreground">{remainingPct}%</span>
                <span className="ml-1 text-muted-foreground">
                  {account.quota}/{maxQuota}
                </span>
              </>
            )}
          </span>
        </div>
        <PillProgress percent={remainingPct} />
      </div>

      <div className="mt-4 flex items-center gap-0.5 border-t border-border/60 pt-2.5 text-muted-foreground">
        <button
          type="button"
          className="cursor-pointer rounded-md p-1.5 transition hover:bg-secondary hover:text-foreground"
          onClick={onCopyToken}
          title="复制 Token"
        >
          <Copy className="size-4" />
        </button>
        <button
          type="button"
          className="cursor-pointer rounded-md p-1.5 transition hover:bg-secondary hover:text-foreground disabled:opacity-50"
          onClick={onEdit}
          disabled={isUpdating}
          title="编辑状态"
        >
          <Pencil className="size-4" />
        </button>
        <button
          type="button"
          className="cursor-pointer rounded-md p-1.5 transition hover:bg-secondary hover:text-foreground disabled:opacity-50"
          onClick={onRefresh}
          disabled={isRefreshing}
          title="刷新额度"
        >
          <RefreshCw className={cn("size-4", isRefreshing && "animate-spin")} />
        </button>
        <button
          type="button"
          className="ml-auto cursor-pointer rounded-md p-1.5 transition hover:bg-rose-50 hover:text-rose-500 disabled:opacity-50"
          onClick={onDelete}
          disabled={isDeleting}
          title="删除"
        >
          <Trash2 className="size-4" />
        </button>
      </div>
    </div>
  );
}

// 模块级缓存。号池页表格高度可观，重新挂载时若从 [] 起跳，
// 会出现塌缩→撑回的视觉跳动；命中缓存时直接给出已有数据并后台静默刷新。
let cachedAccounts: Account[] | null = null;

function AccountsPageContent() {
  const didLoadRef = useRef(false);
  // 命中模块级缓存时直接拿来当初始 state，避免再次切到号池页时
  // accounts 从 [] 起跳、isLoading=true 让大表格塌缩成 spinner 再撑回。
  const [accounts, setAccountsState] = useState<Account[]>(() => cachedAccounts ?? []);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState<AccountStatus | "all">("all");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState("10");
  const [editingAccount, setEditingAccount] = useState<Account | null>(null);
  const [editStatus, setEditStatus] = useState<AccountStatus>("正常");
  const [isLoading, setIsLoading] = useState(() => cachedAccounts === null);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);
  const [viewMode, setViewMode] = useState<"list" | "grid">("grid");
  // 进度条最小显示时长：即便接口几十 ms 就返回，进度条也至少撑过一次完整滑动，
  // 避免出现"按钮 disabled 闪一下、进度条一闪而过"的视觉抖动。
  const [showProgress, setShowProgress] = useState(false);

  // 写 accounts 同步刷新缓存。
  const setAccounts = (next: Account[]) => {
    cachedAccounts = next;
    setAccountsState(next);
  };

  // 持久化视图偏好
  useEffect(() => {
    const saved = window.localStorage.getItem("accounts.viewMode");
    if (saved === "list" || saved === "grid") {
      setViewMode(saved);
    }
  }, []);
  useEffect(() => {
    window.localStorage.setItem("accounts.viewMode", viewMode);
  }, [viewMode]);

  const loadAccounts = async (silent = false) => {
    if (!silent) {
      setIsLoading(true);
    }
    try {
      const data = await fetchAccounts();
      setAccounts(data.items);
      setSelectedIds((prev) => prev.filter((id) => data.items.some((item) => item.access_token === id)));
    } catch (error) {
      const message = error instanceof Error ? error.message : "加载账户失败";
      if (!silent) toast.error(message);
    } finally {
      if (!silent) {
        setIsLoading(false);
      }
    }
  };

  // 进度条状态机：任一操作中立刻显示，操作结束后再保留 1.2s 让滑动动画走完一程，
  // 避免缓存命中（几十毫秒返回）时进度条一闪而过。
  useEffect(() => {
    const isActive = isLoading || isRefreshing || isDeleting;
    if (isActive) {
      setShowProgress(true);
      return;
    }
    const timer = setTimeout(() => setShowProgress(false), 1200);
    return () => clearTimeout(timer);
  }, [isLoading, isRefreshing, isDeleting]);

  useEffect(() => {
    if (didLoadRef.current) {
      return;
    }
    didLoadRef.current = true;
    // 已有缓存：后台静默刷新，不让大表格塌缩。
    void loadAccounts(cachedAccounts !== null);
  }, []);

  const filteredAccounts = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return accounts.filter((account) => {
      const searchMatched =
        normalizedQuery.length === 0 || (account.email ?? "").toLowerCase().includes(normalizedQuery);
      const typeMatched = typeFilter === "all" || displayAccountType(account) === typeFilter;
      const statusMatched = statusFilter === "all" || account.status === statusFilter;
      return searchMatched && typeMatched && statusMatched;
    });
  }, [accounts, query, statusFilter, typeFilter]);

  const pageCount = Math.max(1, Math.ceil(filteredAccounts.length / Number(pageSize)));
  const safePage = Math.min(page, pageCount);
  const startIndex = (safePage - 1) * Number(pageSize);
  const currentRows = filteredAccounts.slice(startIndex, startIndex + Number(pageSize));
  const allCurrentSelected =
    currentRows.length > 0 && currentRows.every((row) => selectedIds.includes(row.access_token));

  const summary = useMemo(() => {
    const total = accounts.length;
    const active = accounts.filter((item) => item.status === "正常").length;
    const limited = accounts.filter((item) => item.status === "限流").length;
    const abnormal = accounts.filter((item) => item.status === "异常").length;
    const disabled = accounts.filter((item) => item.status === "禁用").length;
    const quota = formatQuotaSummary(accounts);

    return { total, active, limited, abnormal, disabled, quota };
  }, [accounts]);

  const accountTypeOptions = useMemo(
    () => [
      { label: "全部类型", value: "all" },
      ...Array.from(new Set(accounts.map(displayAccountType))).map((type) => ({ label: type, value: type })),
    ],
    [accounts],
  );

  const selectedTokens = useMemo(() => {
    const selectedSet = new Set(selectedIds);
    return accounts.filter((item) => selectedSet.has(item.access_token)).map((item) => item.access_token);
  }, [accounts, selectedIds]);

  const abnormalTokens = useMemo(() => {
    return accounts.filter((item) => item.status === "异常").map((item) => item.access_token);
  }, [accounts]);

  const paginationItems = useMemo(() => {
    const items: (number | "...")[] = [];
    const start = Math.max(1, safePage - 1);
    const end = Math.min(pageCount, safePage + 1);

    if (start > 1) items.push(1);
    if (start > 2) items.push("...");
    for (let current = start; current <= end; current += 1) items.push(current);
    if (end < pageCount - 1) items.push("...");
    if (end < pageCount) items.push(pageCount);

    return items;
  }, [pageCount, safePage]);

  const handleDeleteTokens = async (tokens: string[], deleteMailboxes = false) => {
    if (tokens.length === 0) {
      toast.error("请先选择要删除的账户");
      return;
    }

    setIsDeleting(true);
    try {
      const data = deleteMailboxes ? await deleteAccountsWithOptions(tokens, true) : await deleteAccounts(tokens);
      setAccounts(data.items);
      setSelectedIds((prev) => prev.filter((id) => data.items.some((item) => item.access_token === id)));
      toast.success(`删除 ${data.removed ?? 0} 个账户`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "删除账户失败";
      toast.error(message);
    } finally {
      setIsDeleting(false);
    }
  };

  const handleRefreshAccounts = async (accessTokens: string[]) => {
    if (accessTokens.length === 0) {
      toast.error("没有需要刷新的账户");
      return;
    }

    setIsRefreshing(true);
    try {
      const data = await refreshAccounts(accessTokens);
      setAccounts(data.items);
      setSelectedIds((prev) => prev.filter((id) => data.items.some((item) => item.access_token === id)));
      if (data.errors.length > 0) {
        const firstError = data.errors[0]?.error;
        toast.error(
          `刷新成功 ${data.refreshed} 个，失败 ${data.errors.length} 个${firstError ? `，首个错误：${firstError}` : ""}`,
        );
      } else {
        toast.success(`刷新成功 ${data.refreshed} 个账户`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "刷新账户失败";
      toast.error(message);
    } finally {
      setIsRefreshing(false);
    }
  };

  const openEditDialog = (account: Account) => {
    setEditingAccount(account);
    setEditStatus(account.status);
  };

  const handleUpdateAccount = async () => {
    if (!editingAccount) {
      return;
    }

    setIsUpdating(true);
    try {
      const data = await updateAccount(editingAccount.access_token, {
        status: editStatus,
      });
      setAccounts(data.items);
      setSelectedIds((prev) => prev.filter((id) => data.items.some((item) => item.access_token === id)));
      setEditingAccount(null);
      toast.success("账号信息已更新");
    } catch (error) {
      const message = error instanceof Error ? error.message : "更新账号失败";
      toast.error(message);
    } finally {
      setIsUpdating(false);
    }
  };

  const toggleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedIds((prev) => Array.from(new Set([...prev, ...currentRows.map((item) => item.access_token)])));
      return;
    }
    setSelectedIds((prev) => prev.filter((id) => !currentRows.some((row) => row.access_token === id)));
  };

  return (
    <>
      {/* 顶部进度条：任一刷新/删除操作期间显示，并保证至少滑过一程，
          避免缓存命中时一闪而过。1.5px 高度 + 主色渐变胶囊滑动。 */}
      {showProgress ? (
        <div
          aria-hidden
          className="pointer-events-none fixed inset-x-0 top-0 z-50 h-[3px] overflow-hidden bg-stone-100/40"
        >
          <div
            className="animate-top-progress absolute top-0 left-0 h-full w-1/3 rounded-full"
            style={{
              background:
                "linear-gradient(90deg, transparent 0%, oklch(0.55 0.18 258 / 0.95) 50%, transparent 100%)",
            }}
          />
        </div>
      ) : null}

      <section className="mt-4 flex flex-col gap-4 sm:mt-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-1">
          <div className="text-xs font-semibold tracking-[0.18em] text-stone-500 uppercase">
            Account Pool
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">号池管理</h1>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            className="h-10 rounded-xl border-stone-200 bg-white/80 px-4 text-stone-700 hover:bg-white"
            onClick={() => void loadAccounts()}
            disabled={showProgress}
          >
            <RefreshCw className="size-4" />
            刷新
          </Button>
          <Button
            variant="outline"
            className="h-10 rounded-xl border-stone-200 bg-white/80 px-4 text-stone-700 hover:bg-white"
            onClick={() => void handleRefreshAccounts(accounts.map((item) => item.access_token))}
            disabled={showProgress || accounts.length === 0}
          >
            <RefreshCw className="size-4" />
            一键刷新所有账号信息和额度
          </Button>
          <AccountImportDialog
            disabled={showProgress}
            onImported={(items) => {
              setAccounts(items);
              setSelectedIds([]);
              setPage(1);
            }}
          />
          <Button
            variant="outline"
            className="h-10 rounded-xl border-stone-200 bg-white/80 px-4 text-stone-700 hover:bg-white"
            onClick={() => downloadTokens(accounts)}
            disabled={accounts.length === 0}
          >
            <Download className="size-4" />
            导出全部 Token
          </Button>
        </div>
      </section>

      <Dialog open={Boolean(editingAccount)} onOpenChange={(open) => (!open ? setEditingAccount(null) : null)}>
        <DialogContent showCloseButton={false} className="rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>编辑账户</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              手动修改账号状态。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">状态</label>
              <Select value={editStatus} onValueChange={(value) => setEditStatus(value as AccountStatus)}>
                <SelectTrigger className="h-11 rounded-xl border-stone-200 bg-white">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {accountStatusOptions
                    .filter((option) => option.value !== "all")
                    .map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter className="pt-2">
            <Button
              variant="secondary"
              className="h-10 rounded-xl bg-stone-100 px-5 text-stone-700 hover:bg-stone-200"
              onClick={() => setEditingAccount(null)}
              disabled={isUpdating}
            >
              取消
            </Button>
            <Button
              className="h-10 rounded-xl bg-stone-950 px-5 text-white hover:bg-stone-800"
              onClick={() => void handleUpdateAccount()}
              disabled={isUpdating}
            >
              {isUpdating ? <LoaderCircle className="size-4 animate-spin" /> : null}
              保存修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <section className="mt-3 space-y-3 lg:mt-2">
        <div className="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
          {metricCards.map((item) => {
            const Icon = item.icon;
            const value = summary[item.key];
            return (
              <Card key={item.key} className="rounded-xl border border-border bg-card shadow-sm transition hover:border-primary/30 hover:shadow-md">
                <CardContent className="p-4">
                  <div className="mb-3 flex items-start justify-between">
                    <span className="text-xs font-medium text-muted-foreground">{item.label}</span>
                    <Icon className={cn("size-4", item.color)} />
                  </div>
                  <div className={cn("font-data tabular-nums text-[1.85rem] font-semibold tracking-tight", item.color)}>
                    {typeof value === "number" ? formatCompact(value) : value}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      </section>

      <section className="mt-8 space-y-4 sm:mt-10">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-center gap-3">
            <h2 className="text-lg font-semibold tracking-tight">账户列表</h2>
            <Badge variant="secondary" className="rounded-lg bg-stone-200 px-2 py-0.5 text-stone-700">
              {filteredAccounts.length}
            </Badge>
          </div>

          <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
            <div className="relative min-w-[200px] flex-1 sm:flex-initial">
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-stone-400" />
              <Input
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setPage(1);
                }}
                placeholder="搜索邮箱"
                className="h-10 rounded-xl border-stone-200 bg-white/85 pl-10"
              />
            </div>
            <Select
              value={typeFilter}
              onValueChange={(value) => {
                setTypeFilter(value);
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full rounded-xl border-stone-200 bg-white/85 sm:w-[140px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {accountTypeOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={statusFilter}
              onValueChange={(value) => {
                setStatusFilter(value as AccountStatus | "all");
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full rounded-xl border-stone-200 bg-white/85 sm:w-[140px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {accountStatusOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="inline-flex h-10 shrink-0 items-center rounded-xl border border-border bg-secondary/40 p-1">
              <button
                type="button"
                aria-pressed={viewMode === "grid"}
                className={cn(
                  "inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition",
                  viewMode === "grid"
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground",
                )}
                onClick={() => setViewMode("grid")}
                title="卡片视图"
              >
                <LayoutGrid className="size-3.5" />
                卡片
              </button>
              <button
                type="button"
                aria-pressed={viewMode === "list"}
                className={cn(
                  "inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition",
                  viewMode === "list"
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground",
                )}
                onClick={() => setViewMode("list")}
                title="列表视图"
              >
                <Rows3 className="size-3.5" />
                列表
              </button>
            </div>
          </div>
        </div>

        {isLoading && accounts.length === 0 ? (
          <Card className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
            <CardContent className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
              <div className="rounded-xl bg-stone-100 p-3 text-stone-500">
                <LoaderCircle className="size-5 animate-spin" />
              </div>
              <div className="space-y-1">
                <p className="text-sm font-medium text-stone-700">正在加载账户</p>
                <p className="text-sm text-stone-500">从后端同步账号列表和状态。</p>
              </div>
            </CardContent>
          </Card>
        ) : null}

        <Card
          className={cn(
            "overflow-hidden rounded-2xl border-white/80 bg-white/90 shadow-sm",
            isLoading && accounts.length === 0 ? "hidden" : "",
          )}
        >
          <CardContent className="space-y-0 p-0">
            <div className="flex flex-col gap-3 border-b border-stone-100 px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex flex-wrap items-center gap-2 text-sm text-stone-500">
                <Button
                  variant="ghost"
                  className="h-8 rounded-lg px-3 text-stone-500 hover:bg-stone-100"
                  onClick={() => void handleRefreshAccounts(selectedTokens)}
                  disabled={selectedTokens.length === 0 || isRefreshing}
                >
                  {isRefreshing ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                  刷新选中账号信息和额度
                </Button>
                <Button
                  variant="ghost"
                  className="h-8 rounded-lg px-3 text-rose-500 hover:bg-rose-50 hover:text-rose-600"
                  onClick={() => void handleDeleteTokens(abnormalTokens, true)}
                  disabled={abnormalTokens.length === 0 || isDeleting}
                >
                  {isDeleting ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                  移除异常账号
                </Button>
                <Button
                  variant="ghost"
                  className="h-8 rounded-lg px-3 text-rose-500 hover:bg-rose-50 hover:text-rose-600"
                  onClick={() => void handleDeleteTokens(selectedTokens)}
                  disabled={selectedTokens.length === 0 || isDeleting}
                >
                  {isDeleting ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                  删除所选
                </Button>
                {selectedIds.length > 0 ? (
                  <span className="rounded-lg bg-stone-100 px-2.5 py-1 text-xs font-medium text-stone-600">
                    已选择 {selectedIds.length} 项
                  </span>
                ) : null}
                {viewMode === "grid" ? (
                  <label className="ml-auto inline-flex cursor-pointer items-center gap-2 rounded-lg border border-border bg-background px-2.5 py-1 text-xs text-muted-foreground transition hover:text-foreground">
                    <Checkbox
                      checked={allCurrentSelected}
                      onCheckedChange={(checked) => toggleSelectAll(Boolean(checked))}
                    />
                    <span>全选当前页</span>
                  </label>
                ) : null}
              </div>
            </div>

            {viewMode === "list" ? (
              <div className="overflow-x-auto">
              <table className="w-full min-w-[920px] text-left">
                <thead className="border-b border-border bg-secondary/40 text-[12px] font-medium text-muted-foreground">
                  <tr>
                    <th className="w-12 px-4 py-2.5">
                      <Checkbox
                        checked={allCurrentSelected}
                        onCheckedChange={(checked) => toggleSelectAll(Boolean(checked))}
                      />
                    </th>
                    <th className="w-56 px-4 py-2.5 font-medium">Token</th>
                    <th className="w-36 px-4 py-2.5 font-medium">类型</th>
                    <th className="w-24 px-4 py-2.5 font-medium">状态</th>
                    <th className="w-56 px-4 py-2.5 font-medium">账号信息</th>
                    <th className="w-24 px-4 py-2.5 font-medium">额度</th>
                    <th className="w-40 px-4 py-2.5 font-medium">恢复时间</th>
                    <th className="w-36 px-4 py-2.5 font-medium">注册时间</th>
                    <th className="w-18 px-4 py-2.5 font-medium">成功</th>
                    <th className="w-18 px-4 py-2.5 font-medium">失败</th>
                    <th className="w-24 px-4 py-2.5 font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {currentRows.map((account) => {
                    const status = statusMeta[account.status];
                    const StatusIcon = status.icon;

                    return (
                      <tr
                        key={account.access_token}
                        className="border-b border-border/60 text-sm text-foreground/80 transition-colors even:bg-secondary/30 hover:bg-secondary/60"
                      >
                        <td className="px-4 py-3">
                          <Checkbox
                            checked={selectedIds.includes(account.access_token)}
                            onCheckedChange={(checked) => {
                              setSelectedIds((prev) =>
                                checked
                                  ? Array.from(new Set([...prev, account.access_token]))
                                  : prev.filter((item) => item !== account.access_token),
                              );
                            }}
                          />
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <span className="font-data text-[12.5px] text-foreground">
                              {maskToken(account.access_token)}
                            </span>
                            <button
                              type="button"
                              className="cursor-pointer rounded-md p-1 text-muted-foreground transition hover:bg-secondary hover:text-foreground"
                              onClick={() => {
                                void navigator.clipboard.writeText(account.access_token);
                                toast.success("token 已复制");
                              }}
                            >
                              <Copy className="size-4" />
                            </button>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-1.5">
                            <Badge variant="secondary" className="rounded-md bg-secondary font-data text-[11px] font-medium text-foreground/80">
                              {displayAccountType(account)}
                            </Badge>
                            <Badge
                              variant="outline"
                              className={cn("rounded-md font-data text-[11px] font-medium", SOURCE_BADGE_CLASS[displayAccountSource(account)])}
                            >
                              {displayAccountSource(account)}
                            </Badge>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <span className="inline-flex items-center gap-2">
                            <span
                              className={cn(
                                "size-2 rounded-full ring-[3px]",
                                account.status === "正常" && "bg-emerald-500 ring-emerald-500/15",
                                account.status === "限流" && "bg-amber-500 ring-amber-500/15",
                                account.status === "异常" && "bg-rose-500 ring-rose-500/15",
                                account.status === "禁用" && "bg-stone-400 ring-stone-400/15",
                              )}
                            />
                            <span className="text-sm font-medium text-foreground">{account.status}</span>
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <div className="font-data text-[12px] leading-5 text-muted-foreground">{account.email ?? "—"}</div>
                        </td>
                        <td className="px-4 py-3">
                          <span className="font-data tabular-nums text-sm font-semibold text-foreground">
                            {formatQuota(account)}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-xs leading-5 text-muted-foreground">
                          {(() => {
                            const restore = formatRestoreAt(account.restore_at);
                            return (
                              <div className="space-y-0.5">
                                {restore.relative ? <div className="font-medium text-foreground">{restore.relative}</div> : null}
                                <div className="font-data">{restore.absolute}</div>
                              </div>
                            );
                          })()}
                        </td>
                        <td className="px-4 py-3 font-data text-xs leading-5 text-muted-foreground">
                          {formatDateTime(account.created_at)}
                        </td>
                        <td className="px-4 py-3 font-data tabular-nums text-emerald-600">{account.success}</td>
                        <td className="px-4 py-3 font-data tabular-nums text-rose-500">{account.fail}</td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-0.5 text-muted-foreground">
                            <button
                              type="button"
                              className="cursor-pointer rounded-md p-1.5 transition hover:bg-secondary hover:text-foreground"
                              onClick={() => openEditDialog(account)}
                              disabled={isUpdating}
                            >
                              <Pencil className="size-4" />
                            </button>
                            <button
                              type="button"
                              className="cursor-pointer rounded-md p-1.5 transition hover:bg-secondary hover:text-foreground"
                              onClick={() => void handleRefreshAccounts([account.access_token])}
                              disabled={isRefreshing}
                            >
                              <RefreshCw className={cn("size-4", isRefreshing ? "animate-spin" : "")} />
                            </button>
                            <button
                              type="button"
                              className="cursor-pointer rounded-md p-1.5 transition hover:bg-rose-50 hover:text-rose-500"
                              onClick={() => void handleDeleteTokens([account.access_token])}
                              disabled={isDeleting}
                            >
                              <Trash2 className="size-4" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            ) : (
              <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
                {currentRows.map((account) => (
                  <AccountCard
                    key={account.access_token}
                    account={account}
                    selected={selectedIds.includes(account.access_token)}
                    onSelectChange={(checked) => {
                      setSelectedIds((prev) =>
                        checked
                          ? Array.from(new Set([...prev, account.access_token]))
                          : prev.filter((item) => item !== account.access_token),
                      );
                    }}
                    onEdit={() => openEditDialog(account)}
                    onRefresh={() => void handleRefreshAccounts([account.access_token])}
                    onDelete={() => void handleDeleteTokens([account.access_token])}
                    onCopyToken={() => {
                      void navigator.clipboard.writeText(account.access_token);
                      toast.success("token 已复制");
                    }}
                    isRefreshing={isRefreshing}
                    isDeleting={isDeleting}
                    isUpdating={isUpdating}
                  />
                ))}
              </div>
            )}

            {!isLoading && currentRows.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
                <div className="rounded-xl bg-stone-100 p-3 text-stone-500">
                  <Search className="size-5" />
                </div>
                <div className="space-y-1">
                  <p className="text-sm font-medium text-stone-700">没有匹配的账户</p>
                  <p className="text-sm text-stone-500">调整筛选条件或搜索关键字后重试。</p>
                </div>
              </div>
            ) : null}

            <div className="border-t border-stone-100 px-4 py-4">
              <div className="flex items-center justify-center gap-3 overflow-x-auto whitespace-nowrap">
                <div className="shrink-0 text-sm text-stone-500">
                显示第 {filteredAccounts.length === 0 ? 0 : startIndex + 1} -{" "}
                {Math.min(startIndex + Number(pageSize), filteredAccounts.length)} 条，共{" "}
                {filteredAccounts.length} 条
                </div>

                <span className="shrink-0 text-sm leading-none text-stone-500">
                  {safePage} / {pageCount} 页
                </span>
                <Select
                  value={pageSize}
                  onValueChange={(value) => {
                    setPageSize(value);
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="h-10 w-[108px] shrink-0 rounded-lg border-stone-200 bg-white text-sm leading-none">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="10">10 / 页</SelectItem>
                    <SelectItem value="20">20 / 页</SelectItem>
                    <SelectItem value="50">50 / 页</SelectItem>
                    <SelectItem value="100">100 / 页</SelectItem>
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  size="icon"
                  className="size-10 shrink-0 rounded-lg border-stone-200 bg-white"
                  disabled={safePage <= 1}
                  onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                >
                  <ChevronLeft className="size-4" />
                </Button>
                {paginationItems.map((item, index) =>
                  item === "..." ? (
                    <span key={`ellipsis-${index}`} className="px-1 text-sm text-stone-400">
                      ...
                    </span>
                  ) : (
                    <Button
                      key={item}
                      variant={item === safePage ? "default" : "outline"}
                      className={cn(
                        "h-10 min-w-10 shrink-0 rounded-lg px-3",
                        item === safePage
                          ? "bg-stone-950 text-white hover:bg-stone-800"
                          : "border-stone-200 bg-white text-stone-700",
                      )}
                      onClick={() => setPage(item)}
                    >
                      {item}
                    </Button>
                  ),
                )}
                <Button
                  variant="outline"
                  size="icon"
                  className="size-10 shrink-0 rounded-lg border-stone-200 bg-white"
                  disabled={safePage >= pageCount}
                  onClick={() => setPage((prev) => Math.min(pageCount, prev + 1))}
                >
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </section>
    </>
  );
}

export default function AccountsPage() {
  const { isCheckingAuth, session } = useAuthGuard(["admin"]);

  if (isCheckingAuth || !session || session.role !== "admin") {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <LoaderCircle className="size-5 animate-spin text-stone-400" />
      </div>
    );
  }

  return <AccountsPageContent />;
}
