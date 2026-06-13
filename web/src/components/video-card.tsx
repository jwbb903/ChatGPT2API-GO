"use client";

import { memo, useEffect, useMemo, useState } from "react";
import { Play } from "lucide-react";

import { httpRequest } from "@/lib/request";
import { cn } from "@/lib/utils";
import type { VideoInfo } from "@/lib/video";

type VideoCardProps = {
  video: VideoInfo;
  label?: string;
  className?: string;
};

function VideoCardBase({ video, label, className }: VideoCardProps) {
  const [playing, setPlaying] = useState(false);
  const [metadata, setMetadata] = useState<Partial<VideoInfo> | null>(null);
  const [metadataFailed, setMetadataFailed] = useState(false);
  const [thumbFailed, setThumbFailed] = useState(false);
  const {
    id,
    title,
    thumbUrl,
    watchUrl,
    embedUrl,
    metadataUrl,
    providerLabel,
    kind,
  } = video;
  const resolvedVideo = useMemo(
    () => ({
      id,
      title,
      thumbUrl,
      watchUrl,
      embedUrl,
      metadataUrl,
      providerLabel,
      kind,
      ...(metadata || {}),
    }),
    [embedUrl, id, kind, metadata, metadataUrl, providerLabel, thumbUrl, title, watchUrl],
  );
  const hasThumb = Boolean(resolvedVideo.thumbUrl) && !thumbFailed;

  useEffect(() => {
    let active = true;
    setMetadata(null);
    setMetadataFailed(false);
    setThumbFailed(false);
    if (!metadataUrl) return;
    void httpRequest<{
      id?: string;
      title?: string;
      thumb_url?: string;
      watch_url?: string;
      embed_url?: string;
    }>(metadataUrl, { redirectOnUnauthorized: false })
      .then((data) => {
        if (!active) return;
        setMetadata({
          id: data.id || id,
          title: data.title || title,
          thumbUrl: data.thumb_url || thumbUrl,
          watchUrl: data.watch_url || watchUrl,
          embedUrl: data.embed_url || embedUrl,
        });
      })
      .catch(() => {
        if (!active) return;
        setMetadata(null);
        setMetadataFailed(true);
      });
    return () => {
      active = false;
    };
  }, [embedUrl, id, metadataUrl, thumbUrl, title, watchUrl]);

  return (
    <span
      className={cn(
        "my-2 inline-flex w-full max-w-[420px] flex-col overflow-hidden rounded-lg border border-border/60 bg-card align-middle shadow-sm",
        className,
      )}
    >
      <span className={cn(
        "relative block aspect-video w-full bg-stone-100 dark:bg-stone-900",
      )}>
        {playing ? (
          resolvedVideo.embedUrl ? (
            <iframe
              src={resolvedVideo.embedUrl}
              title="video"
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
              allowFullScreen
              className="absolute inset-0 h-full w-full border-0"
            />
          ) : null
        ) : (
          <button
            type="button"
            onClick={() => resolvedVideo.embedUrl && setPlaying(true)}
            className={cn(
              "group absolute inset-0 flex h-full w-full items-center justify-center overflow-hidden text-left",
              resolvedVideo.embedUrl ? "cursor-pointer" : "cursor-wait",
            )}
            aria-label="播放视频"
          >
            {hasThumb ? (
              <img
                src={resolvedVideo.thumbUrl}
                alt=""
                loading="lazy"
                decoding="async"
                className="h-full w-full object-cover"
                onError={() => setThumbFailed(true)}
              />
            ) : (
              <span className="absolute right-3 bottom-3 left-3 line-clamp-2 rounded-md bg-background/85 px-2 py-1 text-center text-[12px] leading-5 text-foreground shadow-sm backdrop-blur-sm">
                {resolvedVideo.title || label || "Bilibili 视频"}
                {metadataFailed ? <span className="ml-1 text-muted-foreground">（封面获取失败）</span> : null}
              </span>
            )}
            {hasThumb ? <span className="absolute inset-0 bg-black/20 transition-opacity group-hover:bg-black/30" /> : null}
            <span className={cn(
              "flex items-center justify-center rounded-full bg-white/90 text-foreground shadow-lg transition-transform group-hover:scale-110",
              hasThumb ? "absolute size-12" : "absolute size-10 shrink-0 border border-border/70",
            )}>
              <Play className="ml-0.5 size-5 fill-current" />
            </span>
            {label && hasThumb && label !== resolvedVideo.watchUrl ? (
              <span className="absolute left-2 top-2 rounded-md bg-black/60 px-1.5 py-0.5 font-data text-[10px] font-medium text-white">
                {label}
              </span>
            ) : null}
            {resolvedVideo.title && hasThumb ? (
              <span className="absolute right-2 bottom-2 left-2 line-clamp-2 rounded-md bg-black/60 px-2 py-1 text-left text-[11px] leading-4 text-white">
                {resolvedVideo.title}
              </span>
            ) : null}
          </button>
        )}
      </span>
      <span className="flex items-center justify-between gap-2 px-3 py-2 text-[12px] text-muted-foreground">
        <span className="truncate">{resolvedVideo.providerLabel}</span>
        <a
          href={resolvedVideo.watchUrl}
          target="_blank"
          rel="noreferrer noopener"
          className="shrink-0 text-primary hover:underline"
        >
          打开原站
        </a>
      </span>
    </span>
  );
}

export const VideoCard = memo(
  VideoCardBase,
  (prev, next) =>
    prev.label === next.label &&
    prev.className === next.className &&
    prev.video.id === next.video.id &&
    prev.video.kind === next.video.kind &&
    prev.video.title === next.video.title &&
    prev.video.thumbUrl === next.video.thumbUrl &&
    prev.video.watchUrl === next.video.watchUrl &&
    prev.video.embedUrl === next.video.embedUrl &&
    prev.video.metadataUrl === next.video.metadataUrl &&
    prev.video.providerLabel === next.video.providerLabel,
);
