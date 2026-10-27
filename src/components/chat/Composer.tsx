import * as React from "react";
import {
  ArrowUpIcon,
  ChevronDownIcon,
  PlusIcon,
  Square,
  XIcon,
} from "lucide-react";
import type { ImageAttachment } from "@/types/chat";
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group";

type Props = {
  onSend: (value: string, image?: ImageAttachment) => Promise<void>;
  onStop: () => void;
  isStreaming: boolean;
  centered?: boolean;
  models?: string[];
  selectedModel?: string | null;
  onModelChange?: (model: string) => void;
  guestMode?: boolean;
};

const defaultProviders = [
  {
    name: "OpenAI",
    models: [
      { id: "gpt-5", name: "GPT-5" },
      { id: "gpt-5-mini", name: "GPT-5 mini" },
    ],
  },
  {
    name: "Anthropic",
    models: [
      { id: "claude-sonnet-4-5", name: "Claude Sonnet 4.5" },
      { id: "claude-haiku-4-5", name: "Claude Haiku 4.5" },
    ],
  },
];

const efforts = [
  { id: "low", name: "Low" },
  { id: "medium", name: "Medium" },
  { id: "high", name: "High" },
];

export function Composer({
  onSend,
  onStop,
  isStreaming,
  models,
  selectedModel,
  onModelChange,
  guestMode = false,
}: Props) {
  const [value, setValue] = React.useState("");
  const [image, setImage] = React.useState<ImageAttachment>();
  const [imageError, setImageError] = React.useState("");
  const [effort, setEffort] = React.useState(efforts[1].id);
  const [localModel, setLocalModel] = React.useState(
    defaultProviders[0].models[0].id
  );

  const formRef = React.useRef<HTMLFormElement>(null);
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const inputRef = React.useRef<HTMLInputElement>(null);
  const imageReadId = React.useRef(0);

  const activeProviders = React.useMemo(() => {
    if (models && models.length > 0) {
      return [
        {
          name: "Ollama",
          models: models.map((m) => ({ id: m, name: m })),
        },
      ];
    }
    return guestMode ? [] : defaultProviders;
  }, [models, guestMode]);

  const allModels = React.useMemo(
    () => activeProviders.flatMap((provider) => provider.models),
    [activeProviders]
  );

  const activeModelId = selectedModel ?? localModel;
  const currentModel =
    allModels.find((item) => item.id === activeModelId) ??
    allModels[0] ?? { id: "default", name: guestMode ? "Model tamu" : "Pilih model" };
  const currentEffort =
    efforts.find((item) => item.id === effort) ?? efforts[1];

  const send = async () => {
    if (isStreaming || (!value.trim() && !image)) return;
    try {
      await onSend(value.trim(), image);
      imageReadId.current++;
      setValue("");
      setImage(undefined);
      setImageError("");
      if (inputRef.current) inputRef.current.value = "";
    } catch {
      /* Keep draft if send fails */
    }
  };

  const processFile = (file: File) => {
    const readId = ++imageReadId.current;
    let type = (file.type || "").toLowerCase();
    if (!type && file.name) {
      if (/\.jpe?g$/i.test(file.name)) type = "image/jpeg";
      else if (/\.png$/i.test(file.name)) type = "image/png";
      else if (/\.webp$/i.test(file.name)) type = "image/webp";
    }
    if (type === "image/jpg") type = "image/jpeg";
    if (!type) type = "image/png";

    if (!["image/jpeg", "image/png", "image/webp"].includes(type)) {
      setImageError("Format gambar harus JPEG, PNG, atau WebP.");
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      setImageError("Ukuran gambar maksimal 5 MB.");
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (readId !== imageReadId.current) return;
      if (typeof reader.result === "string") {
        setImage({
          name: file.name || "image.png",
          type: type as ImageAttachment["type"],
          dataUrl: reader.result,
        });
        setImageError("");
      } else {
        setImageError("Gambar gagal dibaca. Coba lagi.");
      }
    };
    reader.onerror = () => {
      if (readId === imageReadId.current) {
        setImageError("Gambar gagal dibaca. Coba lagi.");
      }
    };
    reader.readAsDataURL(file);
  };

  const readImage = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    processFile(file);
    event.target.value = "";
  };

  const handlePaste = React.useCallback(
    (event: React.ClipboardEvent | ClipboardEvent) => {
      const clipboardData =
        "clipboardData" in event ? event.clipboardData : null;
      if (!clipboardData) return;

      const files = clipboardData.files;
      if (files && files.length > 0) {
        for (let i = 0; i < files.length; i++) {
          const file = files[i];
          if (
            file.type.startsWith("image/") ||
            /\.(jpe?g|png|webp|gif|bmp)$/i.test(file.name)
          ) {
            event.preventDefault();
            processFile(file);
            return;
          }
        }
      }

      const items = clipboardData.items;
      if (items) {
        for (let i = 0; i < items.length; i++) {
          const item = items[i];
          if (item.type.startsWith("image/") || item.kind === "file") {
            const file = item.getAsFile();
            if (file) {
              event.preventDefault();
              processFile(file);
              return;
            }
          }
        }
      }
    },
    []
  );

  React.useEffect(() => {
    const onWindowPaste = (e: ClipboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target && target.tagName === "INPUT" && target !== inputRef.current)
        return;
      handlePaste(e);
    };
    window.addEventListener("paste", onWindowPaste);
    return () => window.removeEventListener("paste", onWindowPaste);
  }, [handlePaste]);

  React.useEffect(() => {
    const input = textareaRef.current;
    if (input) {
      input.style.height = "auto";
      input.style.height = `${Math.min(input.scrollHeight, 200)}px`;
    }
  }, [value]);

  return (
    <form
      ref={formRef}
      onPaste={handlePaste}
      onSubmit={(event) => {
        event.preventDefault();
        void send();
      }}
      className="w-full px-4 pb-3"
    >
      {!guestMode && <input
        ref={inputRef}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        className="sr-only"
        tabIndex={-1}
        onChange={readImage}
      />}
      <InputGroup className="mx-auto max-w-[920px] rounded-2xl bg-card shadow-md transition-all focus-within:border-primary/50 focus-within:shadow-lg focus-within:ring-4 focus-within:ring-primary/10">
        {!guestMode && image && (
          <InputGroupAddon align="block-start" className="flex-wrap">
            <Attachment size="sm">
              <AttachmentMedia variant="image">
                <img
                  src={image.dataUrl}
                  alt={image.name}
                  className="h-full w-full object-cover"
                />
              </AttachmentMedia>
              <AttachmentContent>
                <AttachmentTitle>{image.name}</AttachmentTitle>
              </AttachmentContent>
              <AttachmentActions>
                <AttachmentAction
                  aria-label={`Hapus ${image.name}`}
                  onClick={() => {
                    imageReadId.current++;
                    setImage(undefined);
                    if (inputRef.current) inputRef.current.value = "";
                  }}
                >
                  <XIcon />
                </AttachmentAction>
              </AttachmentActions>
            </Attachment>
          </InputGroupAddon>
        )}
        <InputGroupTextarea
          ref={textareaRef}
          value={value}
          maxLength={8000}
          rows={1}
          aria-label="Tulis pesan"
          placeholder="Ketik pesan..."
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              void send();
            }
          }}
          onPaste={guestMode ? undefined : handlePaste}
          className="p-3"
        />
        <InputGroupAddon align="block-end">
          {!guestMode && <InputGroupButton
            size="icon-sm"
            variant="outline"
            aria-label="Lampirkan gambar"
            onClick={() => inputRef.current?.click()}
            disabled={isStreaming}
            className="rounded-full"
          >
            <PlusIcon />
          </InputGroupButton>}
          {!guestMode && <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <InputGroupButton
                size="sm"
                variant="outline"
                aria-label={`Model: ${currentModel.name}`}
                className="ml-auto"
              >
                {currentModel.name}
                <ChevronDownIcon />
              </InputGroupButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-48">
              <DropdownMenuRadioGroup
                value={activeModelId}
                onValueChange={(val) => {
                  if (onModelChange) {
                    onModelChange(val);
                  } else {
                    setLocalModel(val);
                  }
                }}
              >
                {activeProviders.map((provider, index) => (
                  <React.Fragment key={provider.name}>
                    {index > 0 && <DropdownMenuSeparator />}
                    <DropdownMenuGroup>
                      <DropdownMenuLabel>{provider.name}</DropdownMenuLabel>
                      {provider.models.map((item) => (
                        <DropdownMenuRadioItem key={item.id} value={item.id}>
                          {item.name}
                        </DropdownMenuRadioItem>
                      ))}
                    </DropdownMenuGroup>
                  </React.Fragment>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <InputGroupButton
                size="sm"
                variant="outline"
                aria-label={`Effort: ${currentEffort.name}`}
              >
                <span className="text-muted-foreground">Effort</span>
                {currentEffort.name}
                <ChevronDownIcon />
              </InputGroupButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-36">
              <DropdownMenuGroup>
                <DropdownMenuLabel>Effort</DropdownMenuLabel>
                <DropdownMenuRadioGroup
                  value={effort}
                  onValueChange={setEffort}
                >
                  {efforts.map((item) => (
                    <DropdownMenuRadioItem key={item.id} value={item.id}>
                      {item.name}
                    </DropdownMenuRadioItem>
                  ))}
                </DropdownMenuRadioGroup>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
          {isStreaming ? (
            <InputGroupButton
              type="button"
              size="icon-sm"
              variant="default"
              aria-label="Hentikan"
              onClick={onStop}
              className="rounded-full"
            >
              <Square className="fill-current" />
            </InputGroupButton>
          ) : (
            <InputGroupButton
              type="submit"
              size="icon-sm"
              variant="default"
              aria-label="Kirim"
              disabled={!value.trim() && !image}
              className="rounded-full"
            >
              <ArrowUpIcon />
            </InputGroupButton>
          )}
        </InputGroupAddon>
        {imageError && (
          <p role="alert" className="px-4 pb-2 text-sm text-destructive">
            {imageError}
          </p>
        )}
      </InputGroup>
      <p className="mt-2 text-center text-[11px] text-muted-foreground">
        AI dapat membuat kesalahan. Periksa informasi penting.
      </p>
    </form>
  );
}
