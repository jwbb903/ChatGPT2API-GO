"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ImageIcon, LoaderCircle, MessageSquarePlus, Paperclip, Plus, RefreshCw, Send, Sparkles, StopCircle, Trash2, X } from "lucide-react";
import ReactMarkdown, { type Components } from "react-markdown";
import rehypeHighlight from "rehype-highlight";
import remarkGfm from "remark-gfm";
import { toast } from "sonner";

import "highlight.js/styles/github-dark.css";

import { ImageLightbox } from "@/components/image-lightbox";
import { VideoCard } from "@/components/video-card";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  fetchChatAccountTypes,
  streamChat,
  type ChatAccountType,
  type ChatPersistedMessage,
  type ChatStreamMessage,
} from "@/lib/api";
import { useAuthGuard } from "@/lib/use-auth-guard";
import { cn } from "@/lib/utils";
import { parseVideoUrl } from "@/lib/video";
import { useChatConversationsStore } from "@/store/chat-conversations";

type ChatMessage = {
  id: string;
  role: "user" | "assistant";
  content: string;
  status: "idle" | "streaming" | "error";
  kind?: "text" | "image";
  error?: string;
};

type ChatLightboxImage = {
  id: string;
  src: string;
  sizeLabel?: string;
  dimensions?: string;
};

type ChatUpload = {
  id: string;
  name: string;
  dataUrl: string;
};

const CHAT_ACCOUNT_TYPE_AUTO = "__auto__";
const CHAT_ACCOUNT_TYPE_LABELS: Record<string, string> = {
  free: "Free",
  Plus: "Plus",
  Pro: "Pro",
  ProLite: "ProLite",
  Team: "Team",
  Enterprise: "Enterprise",
};
const CHAT_SCROLL_BOTTOM_THRESHOLD = 80;
const STREAM_BASE_CHARS_PER_SECOND = 72;
const STREAM_CATCHUP_CHARS_PER_SECOND = 220;
const STREAM_FAST_CATCHUP_CHARS_PER_SECOND = 420;
const STREAM_MAX_CHARS_PER_FRAME = 18;

const IMAGE_ACTION_RE =
  /(画|绘制|生成|做|设计|创作|制作).{0,20}(图|图片|图像|海报|头像|插画|壁纸|封面|logo|标志)|(图|图片|图像|海报|头像|插画|壁纸|封面|logo|标志).{0,20}(画|绘制|生成|做|设计|创作|制作)/i;
const DRAW_ACTION_RE = /(^|[\s，。！？,.!?])(帮我|请|给我|帮|麻烦你)?(画|绘制)(一张|一幅|一个|个|张|幅|下|一下)?\S+/i;
const IMAGE_DISCUSSION_RE =
  /(怎么|如何|为什么|教程|步骤|方法|接口|api|代码|报错|失败|问题|原理|区别|能不能|可以吗|会不会|是什么|什么意思).{0,24}(画|绘制|生成|做|设计|创作|制作|图|图片|图像|海报|头像|插画|壁纸|封面|logo|标志)|(画图|绘图|生图|图片生成|图像生成|gpt-image|logo|头像).{0,24}(怎么|如何|为什么|教程|步骤|方法|接口|api|代码|报错|失败|问题|原理|区别|能不能|可以吗|会不会|是什么|什么意思)/i;

function isLikelyImagePrompt(text: string): boolean {
  if (IMAGE_DISCUSSION_RE.test(text)) return false;
  return IMAGE_ACTION_RE.test(text) || DRAW_ACTION_RE.test(text);
}

function formatChatAccountType(value: string): string {
  const trimmed = value.trim();
  return CHAT_ACCOUNT_TYPE_LABELS[trimmed] || trimmed;
}

function isChatViewportNearBottom(viewport: HTMLDivElement): boolean {
  return viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= CHAT_SCROLL_BOTTOM_THRESHOLD;
}

function takeStreamChunk(text: string, count: number): [string, string] {
  if (!text || count <= 0) return ["", text];
  const chars = Array.from(text);
  return [chars.slice(0, count).join(""), chars.slice(count).join("")];
}

function extractPlainText(node: unknown): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(extractPlainText).join("");
  if (node && typeof node === "object" && "props" in node) {
    return extractPlainText((node as { props: { children?: unknown } }).props.children);
  }
  return "";
}

const MARKDOWN_IMAGE_RE = /!\[[^\]]*]\(([^)\s]+)\)/g;

function extractMarkdownImageUrls(content: string): string[] {
  const urls: string[] = [];
  for (const match of content.matchAll(MARKDOWN_IMAGE_RE)) {
    const src = String(match[1] || "").trim();
    if (src) urls.push(src);
  }
  return urls;
}

function isImageOnlyMarkdown(content: string): boolean {
  const urls = extractMarkdownImageUrls(content);
  if (urls.length === 0) return false;
  return content.replace(MARKDOWN_IMAGE_RE, "").trim().length === 0;
}

function buildChatLightboxImages(messages: ChatMessage[]): ChatLightboxImage[] {
  return messages.flatMap((message) => {
    if (message.role !== "assistant" || !message.content) return [];
    return extractMarkdownImageUrls(message.content).map((src, index) => ({
      id: `${message.id}-${index}`,
      src,
    }));
  });
}

function buildAssistantMarkdownComponents(
  onOpenImage?: (src: string) => void,
  options: { renderVideoCards?: boolean } = {},
): Components {
  const renderVideoCards = options.renderVideoCards !== false;
  return {
    a({ href, children, node: _node, ...rest }) {
      const video = renderVideoCards && href ? parseVideoUrl(String(href)) : null;
      if (video) {
        const label = extractPlainText(children).replace(/^\[+|\]+$/g, "").trim() || undefined;
        return <VideoCard video={video} label={label} />;
      }
      return (
        <a href={href} target="_blank" rel="noreferrer noopener" {...rest}>
          {children}
        </a>
      );
    },
    img({ src, alt }) {
      if (!src) return null;
      const imageSrc = String(src);
      return (
        <button
          type="button"
          className="group my-2 block size-36 cursor-zoom-in overflow-hidden rounded-xl border border-border/60 bg-background p-0 shadow-sm sm:size-40"
          onClick={() => onOpenImage?.(imageSrc)}
          aria-label="打开图片预览"
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={imageSrc}
            alt={alt || "generated image"}
            className="block h-full w-full object-cover transition duration-200 group-hover:scale-[1.02] group-hover:brightness-95"
          />
        </button>
      );
    },
  };
}

function AssistantMarkdown({
  content,
  onOpenImage,
  renderVideoCards = true,
}: {
  content: string;
  onOpenImage?: (src: string) => void;
  renderVideoCards?: boolean;
}) {
  const components = buildAssistantMarkdownComponents(onOpenImage, { renderVideoCards });
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[[rehypeHighlight, { detect: true, ignoreMissing: true }]]}
      components={components}
    >
      {content}
    </ReactMarkdown>
  );
}

function ChatImageGeneratingPlaceholder() {
  return (
    <div className="relative h-28 w-56 max-w-full overflow-hidden rounded-xl bg-stone-100/80 dark:bg-stone-900/60">
      <div aria-hidden className="dot-grid-loader absolute inset-0" />
      <div className="absolute top-2 left-3 text-[11px] font-medium text-stone-500 dark:text-stone-400">
        正在创建图片
      </div>
    </div>
  );
}

function ChatThinkingStatus() {
  return (
    <div className="flex items-center gap-2 text-[13px] leading-5 text-muted-foreground">
      <LoaderCircle className="size-4 shrink-0 animate-spin" />
      <span>正在思考...</span>
    </div>
  );
}

function ChatStreamingCaret() {
  return <span className="chat-stream-caret" aria-hidden />;
}

function ChatImageThumbnails({ content, onOpenImage }: { content: string; onOpenImage: (src: string) => void }) {
  const urls = extractMarkdownImageUrls(content);
  if (urls.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-2">
      {urls.map((src, index) => (
        <button
          key={`${src}-${index}`}
          type="button"
          className="group block size-36 cursor-zoom-in overflow-hidden rounded-xl border border-border/60 bg-background p-0 shadow-sm sm:size-40"
          onClick={() => onOpenImage(src)}
          aria-label="打开图片预览"
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={src}
            alt={`generated image ${index + 1}`}
            className="block h-full w-full object-cover transition duration-200 group-hover:scale-[1.02] group-hover:brightness-95"
          />
        </button>
      ))}
    </div>
  );
}

const EMPTY_PROMPTS = [
  "画一张雨夜霓虹街景",
  "帮我优化这段提示词",
  "生成一个圆润的应用图标",
];

function ChatHistoryEmptyState() {
  return (
    <div className="flex h-full min-h-[220px] flex-col items-center justify-center px-5 text-center">
      <div className="relative mb-4">
        <div className="grid size-10 place-items-center rounded-xl border border-border/70 bg-background shadow-sm">
          <MessageSquarePlus className="size-4 text-muted-foreground" />
        </div>
        <span className="absolute -right-1 -bottom-1 grid size-4 place-items-center rounded-full bg-foreground text-background">
          <Sparkles className="size-2.5" />
        </span>
      </div>
      <div className="text-[13px] font-medium text-foreground">还没有历史会话</div>
      <div className="mt-1 max-w-[180px] text-[11px] leading-5 text-muted-foreground">
        新的对话会自动保存在这里，方便继续追问和复用图片上下文。
      </div>
    </div>
  );
}

function ChatEmptyState({ onPickPrompt }: { onPickPrompt: (prompt: string) => void }) {
  return (
    <div className="flex h-full items-center justify-center px-4 py-8">
      <div className="w-full max-w-[520px] text-center">
        <div className="mx-auto mb-5 grid size-14 place-items-center rounded-2xl border border-border/70 bg-background shadow-[0_18px_50px_-28px_rgba(15,23,42,0.45)]">
          <ImageIcon className="size-5 text-foreground" />
        </div>
        <div className="text-[15px] font-semibold tracking-tight text-foreground">开始新的创作对话</div>
        <p className="mx-auto mt-2 max-w-[390px] text-[12px] leading-6 text-muted-foreground">
          可以直接聊天，也可以让它生成图片；对话里的图片支持点开预览，并能在后续追问中被引用。
        </p>
        <div className="mt-5 flex flex-wrap justify-center gap-2">
          {EMPTY_PROMPTS.map((prompt) => (
            <button
              key={prompt}
              type="button"
              className="rounded-full border border-border/70 bg-background px-3 py-1.5 text-[12px] text-foreground shadow-sm transition hover:border-foreground/30 hover:bg-secondary"
              onClick={() => onPickPrompt(prompt)}
            >
              {prompt}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

function createId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

// 第一句用户输入截前 24 个字做标题，做不到的退回到“新对话”。
function deriveTitle(messages: ChatPersistedMessage[]): string {
  const firstUser = messages.find((m) => m.role === "user");
  const text = (firstUser?.content || "").trim().replace(/\s+/g, " ");
  if (!text) return "新对话";
  return text.length > 24 ? `${text.slice(0, 24)}…` : text;
}

function toPersistedMessages(messages: ChatMessage[]): ChatPersistedMessage[] {
  return messages
    .filter((m) => m.status !== "error" && m.content.trim())
    .map((m) => ({ role: m.role, content: m.content }));
}

function fromPersistedMessages(messages: ChatPersistedMessage[]): ChatMessage[] {
  return messages.map((m) => ({
    id: createId(),
    role: m.role === "assistant" ? "assistant" : "user",
    content: m.content,
    status: "idle" as const,
  }));
}

function formatTime(value: number): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
    .format(date);
}

function ChatPageContent() {
  const items = useChatConversationsStore((state) => state.items);
  const isLoadingList = useChatConversationsStore((state) => state.isLoading);
  const hasLoaded = useChatConversationsStore((state) => state.hasLoaded);
  const loadConversations = useChatConversationsStore((state) => state.load);
  const saveConversation = useChatConversationsStore((state) => state.save);
  const removeConversation = useChatConversationsStore((state) => state.remove);

  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [uploads, setUploads] = useState<ChatUpload[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [conversationId, setConversationId] = useState<string>("");
  const [activeId, setActiveId] = useState<string>("");
  const [pendingDelete, setPendingDelete] = useState<string>("");
  const [forceSwitchAccount, setForceSwitchAccount] = useState(false);
  const [accountTypes, setAccountTypes] = useState<ChatAccountType[]>([]);
  const [selectedAccountType, setSelectedAccountType] = useState<string>(CHAT_ACCOUNT_TYPE_AUTO);
  const [lightboxOpen, setLightboxOpen] = useState(false);
  const [lightboxIndex, setLightboxIndex] = useState(0);
  const abortRef = useRef<AbortController | null>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const shouldStickToBottomRef = useRef(true);
  const streamFrameRef = useRef<number | null>(null);
  const streamBufferRef = useRef("");
  const streamLastFrameAtRef = useRef(0);
  const streamDrainResolversRef = useRef<Array<() => void>>([]);

  useEffect(() => {
    void loadConversations().catch((error) => {
      const message = error instanceof Error ? error.message : "加载会话失败";
      toast.error(message);
    });
  }, [loadConversations]);

  useEffect(() => {
    let cancelled = false;
    void fetchChatAccountTypes()
      .then(({ items }) => {
        if (cancelled) return;
        const nextTypes = Array.from(new Set((items || []).map((item) => (typeof item === "string" ? item : item.type || "").trim()).filter(Boolean)));
        setAccountTypes(nextTypes);
        setSelectedAccountType((current) =>
          current === CHAT_ACCOUNT_TYPE_AUTO || nextTypes.includes(current) ? current : CHAT_ACCOUNT_TYPE_AUTO,
        );
      })
      .catch((error) => {
        if (cancelled) return;
        const message = error instanceof Error ? error.message : "加载账号类型失败";
        toast.error(message);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const appendStreamText = useCallback((messageId: string, text: string) => {
    if (!text) return;
    setMessages((prev) =>
      prev.map((m) =>
        m.id === messageId
          ? { ...m, content: m.content + text }
          : m,
      ),
    );
  }, []);

  const resolveStreamDrain = useCallback(() => {
    if (streamBufferRef.current || streamFrameRef.current !== null) return;
    const resolvers = streamDrainResolversRef.current.splice(0);
    resolvers.forEach((resolve) => resolve());
  }, []);

  const waitForStreamDrain = useCallback(() => {
    if (!streamBufferRef.current && streamFrameRef.current === null) {
      return Promise.resolve();
    }
    return new Promise<void>((resolve) => {
      streamDrainResolversRef.current.push(resolve);
    });
  }, []);

  const stopStreamPlayback = useCallback(() => {
    if (streamFrameRef.current !== null) {
      window.cancelAnimationFrame(streamFrameRef.current);
      streamFrameRef.current = null;
    }
    streamLastFrameAtRef.current = 0;
  }, []);

  const flushStreamBuffer = useCallback(
    (messageId: string) => {
      stopStreamPlayback();
      const remaining = streamBufferRef.current;
      streamBufferRef.current = "";
      appendStreamText(messageId, remaining);
      resolveStreamDrain();
    },
    [appendStreamText, resolveStreamDrain, stopStreamPlayback],
  );

  const playStreamFrame = useCallback(
    (messageId: string, now: number) => {
      const elapsedMs = streamLastFrameAtRef.current ? Math.min(80, now - streamLastFrameAtRef.current) : 16;
      streamLastFrameAtRef.current = now;
      const pendingLength = Array.from(streamBufferRef.current).length;
      const speed =
        pendingLength > 360
          ? STREAM_FAST_CATCHUP_CHARS_PER_SECOND
          : pendingLength > 120
            ? STREAM_CATCHUP_CHARS_PER_SECOND
            : STREAM_BASE_CHARS_PER_SECOND;
      const nextCount = Math.max(1, Math.min(STREAM_MAX_CHARS_PER_FRAME, Math.ceil((speed * elapsedMs) / 1000)));
      const [nextText, remaining] = takeStreamChunk(streamBufferRef.current, nextCount);
      streamBufferRef.current = remaining;
      appendStreamText(messageId, nextText);
      if (streamBufferRef.current) {
        streamFrameRef.current = window.requestAnimationFrame((nextNow) => playStreamFrame(messageId, nextNow));
      } else {
        streamFrameRef.current = null;
        streamLastFrameAtRef.current = 0;
        resolveStreamDrain();
      }
    },
    [appendStreamText, resolveStreamDrain],
  );

  const queueStreamDelta = useCallback(
    (messageId: string, delta: string) => {
      streamBufferRef.current += delta;
      if (streamFrameRef.current !== null) return;
      streamFrameRef.current = window.requestAnimationFrame((now) => playStreamFrame(messageId, now));
    },
    [playStreamFrame],
  );

  useEffect(() => {
    return () => {
      if (streamFrameRef.current !== null) {
        window.cancelAnimationFrame(streamFrameRef.current);
        streamFrameRef.current = null;
      }
      streamBufferRef.current = "";
      streamLastFrameAtRef.current = 0;
      streamDrainResolversRef.current.splice(0).forEach((resolve) => resolve());
    };
  }, []);

  // 流式输出时只在用户本来贴底的情况下同步跟随，避免 smooth 动画反复叠加导致闪抖。
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    if (!shouldStickToBottomRef.current) return;
    viewport.scrollTo({ top: viewport.scrollHeight, behavior: "auto" });
  }, [messages]);

  const handleStop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    stopStreamPlayback();
    streamBufferRef.current = "";
    streamDrainResolversRef.current.splice(0).forEach((resolve) => resolve());
    setIsStreaming(false);
    setMessages((prev) => prev.map((m) => (m.status === "streaming" ? { ...m, status: "idle" } : m)));
  }, [stopStreamPlayback]);

  const resetSession = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setIsStreaming(false);
    setMessages([]);
    setConversationId("");
    setActiveId("");
    setInput("");
    setForceSwitchAccount(false);
    setLightboxOpen(false);
    setLightboxIndex(0);
    shouldStickToBottomRef.current = true;
    stopStreamPlayback();
    streamBufferRef.current = "";
    streamDrainResolversRef.current.splice(0).forEach((resolve) => resolve());
  }, [stopStreamPlayback]);

  const handleNewChat = useCallback(() => {
    resetSession();
    textareaRef.current?.focus();
  }, [resetSession]);

  const handlePickEmptyPrompt = useCallback((prompt: string) => {
    setInput(prompt);
    textareaRef.current?.focus();
  }, []);

  const handleSelect = useCallback(
    (id: string) => {
      if (id === activeId || isStreaming) return;
      const target = items.find((item) => item.id === id);
      if (!target) return;
      abortRef.current?.abort();
      abortRef.current = null;
      setIsStreaming(false);
      setMessages(fromPersistedMessages(target.messages));
      setConversationId(target.upstream_conversation_id || "");
      setActiveId(target.id);
      setInput("");
      setForceSwitchAccount(false);
    },
    [activeId, isStreaming, items],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      if (pendingDelete) return;
      setPendingDelete(id);
      try {
        await removeConversation(id);
        if (activeId === id) {
          resetSession();
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : "删除失败";
        toast.error(message);
      } finally {
        setPendingDelete("");
      }
    },
    [activeId, pendingDelete, removeConversation, resetSession],
  );

  const handleFileInput = useCallback((files: FileList | null) => {
    if (!files?.length) return;
    const readers = Array.from(files).slice(0, 5).map((file) => new Promise<ChatUpload>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve({ id: createId(), name: file.name, dataUrl: String(reader.result || "") });
      reader.onerror = () => reject(new Error(`读取文件失败: ${file.name}`));
      reader.readAsDataURL(file);
    }));
    Promise.all(readers)
      .then((items) => setUploads((prev) => [...prev, ...items].slice(0, 5)))
      .catch((error) => toast.error(error instanceof Error ? error.message : "读取文件失败"));
  }, []);

  const handleSubmit = useCallback(async () => {
    const prompt = input.trim();
    const attachedUploads = uploads;
    if ((!prompt && attachedUploads.length === 0) || isStreaming) return;

    const isImagePrompt = isLikelyImagePrompt(prompt);
    const attachmentText = attachedUploads.length > 0 ? `\n\n${attachedUploads.map((item) => `[附件: ${item.name}]`).join("\n")}` : "";
    const userMessage: ChatMessage = { id: createId(), role: "user", content: `${prompt || "请分析这些附件。"}${attachmentText}`, status: "idle" };
    const assistantMessage: ChatMessage = {
      id: createId(),
      role: "assistant",
      content: "",
      status: "streaming",
      kind: isImagePrompt ? "image" : "text",
    };
    const baseHistory = messages;
    shouldStickToBottomRef.current = true;
    setMessages([...baseHistory, userMessage, assistantMessage]);
    setInput("");
    setIsStreaming(true);

    const controller = new AbortController();
    abortRef.current = controller;

    const currentContent: ChatStreamMessage["content"] = attachedUploads.length > 0
      ? [
          { type: "text", text: prompt || "请分析这些附件。" },
          ...attachedUploads.map((item) => ({ type: "input_file", file_name: item.name, file_data: item.dataUrl })),
        ]
      : prompt;
    const apiMessages: ChatStreamMessage[] = [
      ...baseHistory
        .filter((m) => m.status !== "error" && m.content.trim())
        .map((m) => ({ role: m.role, content: m.content }) as ChatStreamMessage),
      { role: "user", content: currentContent },
    ];

    const switchAccount = forceSwitchAccount;
    if (switchAccount) {
      setForceSwitchAccount(false);
    }
    const requestedAccountType =
      selectedAccountType === CHAT_ACCOUNT_TYPE_AUTO ? undefined : selectedAccountType;

    const initialPersisted: ChatPersistedMessage[] = [
      ...toPersistedMessages(baseHistory),
      { role: "user", content: prompt },
    ];
    let streamCid = conversationId;
    let savedConversationId = activeId;
    let assistantContent = "";
    let streamFailed = false;

    try {
      const saved = await saveConversation({
        id: activeId || undefined,
        title: deriveTitle(initialPersisted),
        messages: initialPersisted,
        upstream_conversation_id: streamCid || undefined,
      });
      savedConversationId = saved.id;
      setActiveId(saved.id);
    } catch (error) {
      const message = error instanceof Error ? error.message : "会话保存失败";
      toast.error(message);
    }

    try {
      for await (const event of streamChat(
        {
          model: isImagePrompt ? "gpt-image-2" : "auto",
          messages: apiMessages,
          conversation_id: conversationId || undefined,
          force_switch_account: switchAccount || undefined,
          account_type: requestedAccountType,
        },
        controller.signal,
      )) {
        if (event.type === "conversation.id") {
          streamCid = event.upstream_conversation_id || event.conversation_id;
          setConversationId(streamCid);
        } else if (event.type === "delta") {
          if (event.upstream_conversation_id || event.conversation_id) {
            streamCid = event.upstream_conversation_id || event.conversation_id || streamCid;
            setConversationId(streamCid);
          }
          assistantContent += event.text;
          queueStreamDelta(assistantMessage.id, event.text);
        } else if (event.type === "error") {
          throw new Error(event.message);
        } else if (event.type === "done") {
          if (event.upstream_conversation_id || event.conversation_id) {
            streamCid = event.upstream_conversation_id || event.conversation_id || streamCid;
            setConversationId(streamCid);
          }
          await waitForStreamDrain();
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantMessage.id
                ? { ...m, status: "idle" }
                : m,
            ),
          );
        }
      }
    } catch (error) {
      streamFailed = true;
      if (controller.signal.aborted) {
        return;
      }
      const message = error instanceof Error ? error.message : "对话失败";
      setMessages((prev) =>
        prev.map((m) => (m.id === assistantMessage.id ? { ...m, status: "error", error: message } : m)),
      );
      toast.error(message);
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (streamFailed || controller.signal.aborted) {
        flushStreamBuffer(assistantMessage.id);
      } else {
        await waitForStreamDrain();
      }
      setIsStreaming(false);
    }

    if (streamFailed) {
      return;
    }
    if (!assistantContent.trim()) {
      const message = "上游没有返回内容";
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMessage.id
            ? { ...m, status: "error", error: message }
            : m,
        ),
      );
      toast.error(message);
      return;
    }

    // done 之后一次性整条覆盖保存：包含完整历史 + upstream cid，给后端做 token 回填。
    const persisted: ChatPersistedMessage[] = [
      ...toPersistedMessages(baseHistory),
      { role: "user", content: prompt },
      { role: "assistant", content: assistantContent },
    ];
    try {
      const saved = await saveConversation({
        id: savedConversationId || activeId || undefined,
        title: deriveTitle(persisted),
        messages: persisted,
        upstream_conversation_id: streamCid || undefined,
      });
      setActiveId(saved.id);
    } catch (error) {
      const message = error instanceof Error ? error.message : "会话保存失败";
      toast.error(message);
    }
  }, [
    activeId,
    conversationId,
    flushStreamBuffer,
    forceSwitchAccount,
    input,
    isStreaming,
    messages,
    queueStreamDelta,
    saveConversation,
    selectedAccountType,
    waitForStreamDrain,
  ]);

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
        event.preventDefault();
        void handleSubmit();
      }
    },
    [handleSubmit],
  );

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${Math.min(textarea.scrollHeight, 240)}px`;
  }, [input]);

  const sortedItems = useMemo(() => items, [items]);
  const lightboxImages = useMemo(() => buildChatLightboxImages(messages), [messages]);

  const openLightbox = useCallback(
    (src: string) => {
      const index = lightboxImages.findIndex((item) => item.src === src);
      if (index < 0) return;
      setLightboxIndex(index);
      setLightboxOpen(true);
    },
    [lightboxImages],
  );

  return (
    <>
    <section className="relative mx-auto flex h-[calc(100dvh-3.5rem)] min-h-0 w-full max-w-[1180px] gap-3 px-3 pb-[calc(env(safe-area-inset-bottom)+0.5rem)] sm:h-[calc(100dvh-4rem)] sm:px-4 sm:pb-6">
      <aside className="hidden w-[260px] shrink-0 flex-col gap-2 pt-3 md:flex">
        <Button
          variant="outline"
          className="h-9 cursor-pointer justify-start rounded-lg border-border bg-card/90 px-3 text-foreground"
          onClick={handleNewChat}
          disabled={isStreaming}
        >
          <MessageSquarePlus className="size-4" />
          <span className="text-[13px]">新建对话</span>
        </Button>
        <div className="hide-scrollbar min-h-0 flex-1 overflow-y-auto rounded-xl border border-border/50 bg-card/40 p-2">
          {isLoadingList && !hasLoaded ? (
            <div className="flex items-center justify-center py-10">
              <LoaderCircle className="size-4 animate-spin text-muted-foreground" />
            </div>
          ) : sortedItems.length === 0 ? (
            <ChatHistoryEmptyState />
          ) : (
            <ul className="flex flex-col gap-1">
              {sortedItems.map((item) => {
                const isActive = item.id === activeId;
                const isDeleting = pendingDelete === item.id;
                return (
                  <li key={item.id}>
                    <div
                      className={cn(
                        "group flex cursor-pointer items-center gap-2 rounded-lg px-2 py-2 text-[13px] transition-colors",
                        isActive
                          ? "bg-foreground text-background"
                          : "text-foreground hover:bg-secondary",
                      )}
                      onClick={() => handleSelect(item.id)}
                    >
                      <div className="min-w-0 flex-1">
                        <div className="truncate">{item.title || "新对话"}</div>
                        <div
                          className={cn(
                            "mt-0.5 truncate text-[10px]",
                            isActive ? "text-background/70" : "text-muted-foreground/70",
                          )}
                        >
                          {formatTime(item.updated_at)}
                        </div>
                      </div>
                      <button
                        type="button"
                        className={cn(
                          "rounded-md p-1 opacity-0 transition-opacity hover:bg-rose-100 hover:text-rose-600 group-hover:opacity-100",
                          isActive && "opacity-100",
                        )}
                        onClick={(event) => {
                          event.stopPropagation();
                          void handleDelete(item.id);
                        }}
                        disabled={isDeleting}
                        aria-label="删除对话"
                      >
                        {isDeleting ? (
                          <LoaderCircle className="size-3.5 animate-spin" />
                        ) : (
                          <Trash2 className="size-3.5" />
                        )}
                      </button>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:gap-3">
        <div className="flex shrink-0 items-center gap-2 pt-3 md:hidden">
          <Button
            variant="outline"
            className="h-9 cursor-pointer rounded-lg border-border bg-card/90 px-3 text-foreground"
            onClick={handleNewChat}
            disabled={messages.length === 0 && !isStreaming}
          >
            <Plus className="size-4" />
            <span className="text-[13px]">新建对话</span>
          </Button>
          {conversationId ? (
            <span className="ml-auto truncate font-data text-[10px] text-muted-foreground/70">
              cid: {conversationId.slice(0, 8)}
            </span>
          ) : null}
        </div>

        <div
          ref={viewportRef}
          onScroll={(event) => {
            shouldStickToBottomRef.current = isChatViewportNearBottom(event.currentTarget);
          }}
          className="hide-scrollbar mt-3 min-h-0 flex-1 overflow-y-auto overscroll-contain rounded-xl border border-border/50 bg-card/40 p-4 md:mt-3"
        >
          {messages.length === 0 ? (
            <ChatEmptyState onPickPrompt={handlePickEmptyPrompt} />
          ) : (
            <div className="flex flex-col gap-4">
              {messages.map((message) => (
                <div key={message.id} className={cn("flex w-full", message.role === "user" ? "justify-end" : "justify-start")}>
                  <div
                    className={cn(
                      "max-w-[85%] rounded-2xl px-4 py-2.5 text-[14px] leading-6",
                      message.role === "user"
                        ? "bg-foreground text-background"
                        : "bg-secondary text-foreground",
                      message.role === "assistant" && "min-w-[4.5rem]",
                      message.role === "assistant" && message.content && !isImageOnlyMarkdown(message.content) && "w-[85%]",
                      message.role === "assistant" && message.content && isImageOnlyMarkdown(message.content) && "bg-transparent p-0",
                      message.status === "error" && "bg-rose-50 text-rose-700 dark:bg-rose-950/60 dark:text-rose-200",
                    )}
                  >
                    {message.role === "assistant" ? (
                      message.content ? (
                        isImageOnlyMarkdown(message.content) ? (
                          <ChatImageThumbnails content={message.content} onOpenImage={openLightbox} />
                        ) : (
                          <div className="prose prose-sm chat-md max-w-none break-words">
                            <AssistantMarkdown
                              content={message.content}
                              onOpenImage={openLightbox}
                              renderVideoCards={message.status !== "streaming"}
                            />
                            {message.status === "streaming" ? <ChatStreamingCaret /> : null}
                          </div>
                        )
                      ) : message.status === "streaming" ? (
                        message.kind === "image" ? (
                          <ChatImageGeneratingPlaceholder />
                        ) : (
                          <ChatThinkingStatus />
                        )
                      ) : message.status === "error" ? (
                        <span>{message.error || "出错了"}</span>
                      ) : (
                        <span className="text-muted-foreground">（空回复）</span>
                      )
                    ) : (
                      <span className="whitespace-pre-wrap break-words">{message.content}</span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="shrink-0 rounded-xl border border-border/60 bg-background p-2 shadow-sm">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="发送消息，或描述你想画的图..."
            rows={1}
            className="hide-scrollbar w-full resize-none bg-transparent px-2 py-2 text-[14px] leading-6 text-foreground placeholder:text-muted-foreground/70 focus:outline-none"
          />
          <div className="flex items-center justify-between gap-2 pt-1">
            <Select
              value={selectedAccountType}
              onValueChange={setSelectedAccountType}
              disabled={isStreaming || accountTypes.length === 0}
            >
              <SelectTrigger className="h-9 w-[116px] shrink-0 rounded-lg border-border bg-background px-3 text-[12px] shadow-none">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={CHAT_ACCOUNT_TYPE_AUTO}>自动</SelectItem>
                {accountTypes.map((type) => (
                  <SelectItem key={type} value={type}>
                    {formatChatAccountType(type)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="flex items-center justify-end gap-2">
              {conversationId && !isStreaming ? (
                <Button
                  variant="outline"
                  size="icon"
                  className={cn(
                    "size-9 cursor-pointer rounded-lg border-border",
                    forceSwitchAccount && "border-foreground bg-foreground text-background hover:bg-foreground/90 hover:text-background",
                  )}
                  onClick={() => setForceSwitchAccount((prev) => !prev)}
                  title={forceSwitchAccount ? "下一条已切换到新账号（点击取消）" : "下一条切换到其他账号续聊"}
                  aria-pressed={forceSwitchAccount}
                >
                  <RefreshCw className="size-4" />
                </Button>
              ) : null}
              {isStreaming ? (
                <Button variant="outline" className="h-9 cursor-pointer rounded-lg px-3" onClick={handleStop}>
                  <StopCircle className="size-4" />
                  <span className="text-[13px]">停止</span>
                </Button>
              ) : (
                <Button
                  className="h-9 cursor-pointer rounded-lg bg-foreground px-3 text-background hover:bg-foreground/90"
                  onClick={() => void handleSubmit()}
                  disabled={!input.trim()}
                >
                  <Send className="size-4" />
                  <span className="text-[13px]">发送</span>
                </Button>
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
    <ImageLightbox
      images={lightboxImages}
      currentIndex={lightboxIndex}
      open={lightboxOpen}
      onOpenChange={setLightboxOpen}
      onIndexChange={setLightboxIndex}
    />
    </>
  );
}

export default function ChatPage() {
  const { isCheckingAuth, session } = useAuthGuard();

  if (isCheckingAuth || !session) {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <LoaderCircle className="size-5 animate-spin text-stone-400" />
      </div>
    );
  }

  return <ChatPageContent />;
}
