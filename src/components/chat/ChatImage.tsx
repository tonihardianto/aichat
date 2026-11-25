import { Download, ExternalLink } from "lucide-react";

const imageActionClass = "inline-flex size-8 items-center justify-center rounded-md bg-background/80 text-foreground shadow-sm backdrop-blur transition-colors hover:bg-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/** Image with download / open-in-new-tab actions (images are same-origin, so `download` works). */
export function ChatImage({ src, alt, className = "max-h-[32rem]" }: { src?: string; alt?: string; className?: string }) {
  if (!src) return null;
  const filename = (src.split("?")[0].split("/").pop() || "gambar.png").replace(/[^\w.-]/g, "_");
  return (
    <span className="group relative my-2 inline-block max-w-full align-top">
      <img src={src} alt={alt ?? ""} loading="lazy" className={`block max-w-full rounded-xl border object-contain ${className}`} />
      <span className="absolute right-2 top-2 flex gap-1 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100">
        <a href={src} download={filename} aria-label="Unduh gambar" title="Unduh gambar" className={imageActionClass}><Download className="size-4" /></a>
        <a href={src} target="_blank" rel="noopener noreferrer" aria-label="Buka gambar di tab baru" title="Buka di tab baru" className={imageActionClass}><ExternalLink className="size-4" /></a>
      </span>
    </span>
  );
}
