"use client";

import { CalendarDays, CarFront, Crosshair, Hash, Trash2 } from "lucide-react";
import { photoContentUrl } from "@/lib/api";
import { formatDate, shortId } from "@/lib/format";
import type { Observation } from "@/lib/types";

type Props = {
  observation: Observation;
  confidence?: number;
  rank?: number;
  deleting?: boolean;
  onDelete?: (observation: Observation) => void;
};

export function ObservationCard({ observation, confidence, rank, deleting, onDelete }: Props) {
  const score = confidence === undefined ? undefined : Math.max(0, Math.min(100, confidence * 100));

  return (
    <article className="observation-card">
      <div className="observation-card__visual">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={photoContentUrl(observation.photo_id)} alt={observation.image_id ? `Кадр ${observation.image_id}` : "Кадр автомобиля"} loading="lazy" />
        {rank !== undefined && <span className="rank-badge">#{rank}</span>}
        <span className="bbox-badge"><Crosshair size={12} /> {observation.bbox.w} × {observation.bbox.h}</span>
      </div>
      <div className="observation-card__body">
        <div className="observation-card__title-row">
          <div className="observation-card__icon"><CarFront size={18} /></div>
          <div>
            <strong>{observation.vehicle_id || `Объект ${shortId(observation.id)}`}</strong>
            <span>{observation.image_id || `Фото ${shortId(observation.photo_id)}`}</span>
          </div>
          {onDelete && (
            <button className="icon-button icon-button--danger" onClick={() => onDelete(observation)} disabled={deleting} aria-label="Удалить наблюдение">
              <Trash2 size={17} />
            </button>
          )}
        </div>

        {confidence !== undefined && (
          <div className="score">
            <div className="score__row">
              <span>Косинусное сходство</span>
              <strong>{confidence.toFixed(3)}</strong>
            </div>
            <div className="score__track"><span style={{ width: `${score}%` }} /></div>
          </div>
        )}

        <div className="observation-card__meta">
          <span><CalendarDays size={13} /> {formatDate(observation.created_at)}</span>
          <span><Hash size={13} /> {shortId(observation.id)}</span>
        </div>
      </div>
    </article>
  );
}
