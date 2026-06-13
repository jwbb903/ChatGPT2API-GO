"use client";

import * as React from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { DayPicker } from "react-day-picker";

import { cn } from "@/lib/utils";

function Calendar({
  className,
  classNames,
  showOutsideDays = true,
  ...props
}: React.ComponentProps<typeof DayPicker>) {
  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      className={cn("p-1 text-sm", className)}
      classNames={{
        months: "flex flex-col gap-4 sm:flex-row",
        month: "relative",
        month_caption: "flex h-9 items-center justify-center font-medium",
        nav: "absolute inset-x-2 top-2 flex items-center justify-between",
        button_previous: "inline-flex size-8 cursor-pointer items-center justify-center rounded-lg hover:bg-stone-100",
        button_next: "inline-flex size-8 cursor-pointer items-center justify-center rounded-lg hover:bg-stone-100",
        weekdays: "mt-2 grid grid-cols-7 text-xs text-stone-400",
        weekday: "flex h-8 items-center justify-center font-normal",
        week: "grid grid-cols-7",
        day: "relative size-9 p-0 text-center",
        day_button: "size-9 cursor-pointer rounded-lg text-sm transition hover:bg-stone-100 disabled:cursor-not-allowed",
        today: "font-semibold text-stone-950",
        // 单日选中 / 区间端点：按钮变深，文字白。
        selected: "[&>button]:bg-stone-900 [&>button]:text-white [&>button]:hover:bg-stone-800",
        // 区间中段：浅灰色铺满，按钮去圆角且不抢眼，hover 略加深以区分。
        // 用 ! 强制覆盖 selected 的深色背景，避免依赖 Tailwind 类生成顺序。
        range_middle:
          "[&>button]:!rounded-none [&>button]:!bg-stone-100 [&>button]:!text-stone-900 [&>button]:hover:!bg-stone-200",
        // 端点朝向中段那一侧去掉圆角，使端点和中段在视觉上自然拼接。
        range_start: "[&>button]:!rounded-r-none",
        range_end: "[&>button]:!rounded-l-none",
        outside: "text-stone-300",
        disabled: "text-stone-300 opacity-50",
        ...classNames,
      }}
      components={{
        Chevron: ({ orientation }) =>
          orientation === "left" ? <ChevronLeft className="size-4" /> : <ChevronRight className="size-4" />,
      }}
      {...props}
    />
  );
}

export { Calendar };
