"use client";

import { LoaderCircle, RotateCcw, Save } from "lucide-react";

import { Button } from "@/components/ui/button";

import { useSettingsStore } from "../store";

/**
 * 设置页底部浮动保存栏。
 *
 * 行为：
 *   - 只有 isDirty=true 时浮现，干净状态下完全不占视觉位
 *   - 居中固定在底部，sm: max-w-3xl 让宽度跟主内容对齐
 *   - 提供"取消修改"= 重新拉一次 config，把 dirty 全打回去
 *   - 保存按钮 disabled 期间锁住，避免双击连发
 *
 * 跟"每张卡尾巴一个保存按钮"相比的优势：
 *   - 跨多 section 修改时只需一次 commit，不用记得逐张卡保存
 *   - 视觉聚焦：用户改东西时持续看到一条提示"你有未保存的修改"
 */
export function FloatingSaveBar() {
  const isDirty = useSettingsStore((s) => s.isDirty);
  const isSaving = useSettingsStore((s) => s.isSavingConfig);
  const saveConfig = useSettingsStore((s) => s.saveConfig);
  const revertConfig = useSettingsStore((s) => s.revertConfig);

  if (!isDirty) return null;

  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-6 z-40 flex justify-center px-4">
      <div className="pointer-events-auto flex w-full max-w-3xl items-center justify-between gap-3 rounded-2xl border border-stone-200 bg-white/95 px-4 py-3 shadow-lg backdrop-blur">
        <div className="flex items-center gap-2 text-sm text-stone-700">
          <span className="inline-flex size-2 animate-pulse rounded-full bg-amber-500" />
          有未保存的修改
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            className="h-9 cursor-pointer rounded-xl border-stone-200 bg-white px-3 text-stone-700 hover:bg-stone-50"
            onClick={() => void revertConfig()}
            disabled={isSaving}
          >
            <RotateCcw className="size-4" />
            取消修改
          </Button>
          <Button
            className="h-9 cursor-pointer rounded-xl bg-stone-950 px-4 text-white hover:bg-stone-800"
            onClick={() => void saveConfig()}
            disabled={isSaving}
          >
            {isSaving ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}
            保存
          </Button>
        </div>
      </div>
    </div>
  );
}
