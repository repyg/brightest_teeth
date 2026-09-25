"use client";

import { useRef, useState, type PointerEvent } from "react";
import { Crosshair, Maximize2 } from "lucide-react";
import type { BBox } from "@/lib/types";

type Handle = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w";
type DragState = {
  kind: "new" | "move" | "resize";
  startX: number;
  startY: number;
  startBox: BBox;
  handle?: Handle;
};

type Props = {
  src: string;
  width: number;
  height: number;
  bbox: BBox;
  onChange: (bbox: BBox) => void;
};

const handles: Handle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];

export function ImageAnnotator({ src, width, height, bbox, onChange }: Props) {
  const frameRef = useRef<HTMLDivElement>(null);
  const [drag, setDrag] = useState<DragState | null>(null);

  const point = (event: PointerEvent) => {
    const rect = frameRef.current!.getBoundingClientRect();
    return {
      x: Math.max(0, Math.min(width, ((event.clientX - rect.left) / rect.width) * width)),
      y: Math.max(0, Math.min(height, ((event.clientY - rect.top) / rect.height) * height)),
    };
  };

  const clamp = (box: BBox): BBox => {
    const x = Math.round(Math.max(0, Math.min(width - 1, box.x)));
    const y = Math.round(Math.max(0, Math.min(height - 1, box.y)));
    return {
      x,
      y,
      w: Math.round(Math.max(1, Math.min(width - x, box.w))),
      h: Math.round(Math.max(1, Math.min(height - y, box.h))),
    };
  };

  const startNew = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    const p = point(event);
    const next = { x: Math.round(p.x), y: Math.round(p.y), w: 1, h: 1 };
    onChange(next);
    setDrag({ kind: "new", startX: p.x, startY: p.y, startBox: next });
  };

  const startExisting = (event: PointerEvent, kind: "move" | "resize", handle?: Handle) => {
    event.stopPropagation();
    frameRef.current?.setPointerCapture(event.pointerId);
    const p = point(event);
    setDrag({ kind, handle, startX: p.x, startY: p.y, startBox: bbox });
  };

  const move = (event: PointerEvent<HTMLDivElement>) => {
    if (!drag) return;
    const p = point(event);
    const dx = p.x - drag.startX;
    const dy = p.y - drag.startY;

    if (drag.kind === "new") {
      onChange(clamp({
        x: Math.min(drag.startX, p.x),
        y: Math.min(drag.startY, p.y),
        w: Math.max(1, Math.abs(p.x - drag.startX)),
        h: Math.max(1, Math.abs(p.y - drag.startY)),
      }));
      return;
    }

    if (drag.kind === "move") {
      onChange(clamp({
        ...drag.startBox,
        x: Math.max(0, Math.min(width - drag.startBox.w, drag.startBox.x + dx)),
        y: Math.max(0, Math.min(height - drag.startBox.h, drag.startBox.y + dy)),
      }));
      return;
    }

    let { x, y, w, h } = drag.startBox;
    const right = x + w;
    const bottom = y + h;
    if (drag.handle?.includes("w")) { x = Math.min(right - 1, x + dx); w = right - x; }
    if (drag.handle?.includes("e")) w = Math.max(1, Math.min(width - x, w + dx));
    if (drag.handle?.includes("n")) { y = Math.min(bottom - 1, y + dy); h = bottom - y; }
    if (drag.handle?.includes("s")) h = Math.max(1, Math.min(height - y, h + dy));
    onChange(clamp({ x, y, w, h }));
  };

  return (
    <div className="annotator-wrap">
      <div
        ref={frameRef}
        className={`annotator ${drag ? "annotator--dragging" : ""}`}
        style={{ aspectRatio: `${width} / ${height}` }}
        onPointerDown={startNew}
        onPointerMove={move}
        onPointerUp={() => setDrag(null)}
        onPointerCancel={() => setDrag(null)}
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={src} alt="Загруженный кадр" draggable={false} />
        <div className="annotator__shade" />
        <div
          className="bbox"
          style={{
            left: `${(bbox.x / width) * 100}%`,
            top: `${(bbox.y / height) * 100}%`,
            width: `${(bbox.w / width) * 100}%`,
            height: `${(bbox.h / height) * 100}%`,
          }}
          onPointerDown={(event) => startExisting(event, "move")}
        >
          <span className="bbox__label"><Crosshair size={12} /> область поиска</span>
          <span className="bbox__corner bbox__corner--tl" />
          <span className="bbox__corner bbox__corner--tr" />
          <span className="bbox__corner bbox__corner--bl" />
          <span className="bbox__corner bbox__corner--br" />
          {handles.map((handle) => (
            <button
              key={handle}
              type="button"
              className={`bbox__handle bbox__handle--${handle}`}
              aria-label={`Изменить рамку: ${handle}`}
              onPointerDown={(event) => startExisting(event, "resize", handle)}
            />
          ))}
        </div>
        <span className="annotator__size">{width} × {height} px</span>
      </div>
      <button className="ghost-button annotator__full" type="button" onClick={() => onChange({ x: 0, y: 0, w: width, h: height })}>
        <Maximize2 size={15} /> Выбрать весь кадр
      </button>
    </div>
  );
}
