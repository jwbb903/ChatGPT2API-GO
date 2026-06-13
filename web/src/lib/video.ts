export type VideoInfo = {
  kind: "youtube" | "bilibili";
  id: string;
  embedUrl: string;
  thumbUrl?: string;
  watchUrl: string;
  providerLabel: string;
  metadataUrl?: string;
  title?: string;
};

function makeYouTube(id: string): VideoInfo {
  return {
    kind: "youtube",
    id,
    embedUrl: `https://www.youtube-nocookie.com/embed/${id}?autoplay=1&rel=0`,
    thumbUrl: `https://img.youtube.com/vi/${id}/hqdefault.jpg`,
    watchUrl: `https://www.youtube.com/watch?v=${id}`,
    providerLabel: "YouTube",
  };
}

function makeBilibili(id: string, page = "1"): VideoInfo {
  const canonicalId = id.trim().replace(/\/+$/, "");
  const isAv = /^av\d+$/i.test(id);
  const query = isAv
    ? `aid=${encodeURIComponent(canonicalId.slice(2))}&p=${encodeURIComponent(page)}`
    : `bvid=${encodeURIComponent(canonicalId)}&p=${encodeURIComponent(page)}`;
  return {
    kind: "bilibili",
    id: canonicalId,
    embedUrl: `https://player.bilibili.com/player.html?${query}&autoplay=1`,
    watchUrl: `https://www.bilibili.com/video/${canonicalId}`,
    providerLabel: "Bilibili",
    metadataUrl: `/api/video/metadata?url=${encodeURIComponent(`https://www.bilibili.com/video/${canonicalId}?p=${page}`)}`,
  };
}

function makeBilibiliPending(url: string): VideoInfo {
  return {
    kind: "bilibili",
    id: url,
    embedUrl: "",
    watchUrl: url,
    providerLabel: "Bilibili",
    metadataUrl: `/api/video/metadata?url=${encodeURIComponent(url)}`,
  };
}

export function parseVideoUrl(href: string): VideoInfo | null {
  if (!href) return null;
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return null;
  }
  const host = url.hostname.toLowerCase();
  if (host === "youtu.be") {
    const id = url.pathname.replace(/^\//, "").split("/")[0];
    if (id) return makeYouTube(id);
    return null;
  }
  if (host === "youtube.com" || host.endsWith(".youtube.com") || host.endsWith(".youtube-nocookie.com")) {
    const v = url.searchParams.get("v");
    if (v) return makeYouTube(v);
    const segments = url.pathname.split("/").filter(Boolean);
    if (segments.length >= 2 && ["embed", "shorts", "v"].includes(segments[0])) {
      return makeYouTube(segments[1]);
    }
  }
  if (host === "bilibili.com" || host.endsWith(".bilibili.com")) {
    const segments = url.pathname.split("/").filter(Boolean);
    const videoIndex = segments.findIndex((item) => item.toLowerCase() === "video");
    const id = videoIndex >= 0 ? (segments[videoIndex + 1] || "").replace(/\/+$/, "") : "";
    if (/^(BV[0-9A-Za-z]+|av\d+)$/i.test(id)) {
      return makeBilibili(id, url.searchParams.get("p") || url.searchParams.get("page") || "1");
    }
  }
  if (host === "b23.tv" || host === "bili2233.cn") {
    return makeBilibiliPending(href);
  }
  return null;
}
