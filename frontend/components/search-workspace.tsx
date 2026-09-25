"use client";

import { useEffect, useRef, useState, type ChangeEvent, type DragEvent } from "react";
import {
  AlertCircle,
  ArrowRight,
  Braces,
  Check,
  ChevronDown,
  Copy,
  Database,
  ImagePlus,
  Info,
  LoaderCircle,
  Plus,
  RotateCcw,
  Search,
  SlidersHorizontal,
  Sparkles,
  UploadCloud,
  X,
} from "lucide-react";
import { api } from "@/lib/api";
import { errorMessage, formatBytes } from "@/lib/format";
import type { BBox, Feature, Photo, SearchMode, SearchResult } from "@/lib/types";
import { ImageAnnotator } from "./image-annotator";
import { ObservationCard } from "./observation-card";

type Props = {
  health: "checking" | "online" | "offline";
  onGalleryChanged: () => void;
  onOpenGallery: () => void;
};

const emptyBox: BBox = { x: 0, y: 0, w: 1, h: 1 };

export function SearchWorkspace({ health, onGalleryChanged, onOpenGallery }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragActive, setDragActive] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState("");
  const [photo, setPhoto] = useState<Photo | null>(null);
  const [bbox, setBbox] = useState<BBox>(emptyBox);
  const [uploading, setUploading] = useState(false);
  const [busy, setBusy] = useState<"search" | "gallery" | "feature" | null>(null);
  const [mode, setMode] = useState<SearchMode>("candidates");
  const [topK, setTopK] = useState(10);
  const [threshold, setThreshold] = useState(0.36);
  const [imageId, setImageId] = useState("");
  const [vehicleId, setVehicleId] = useState("");
  const [excludeIds, setExcludeIds] = useState("");
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [result, setResult] = useState<SearchResult | null>(null);
  const [feature, setFeature] = useState<Feature | null>(null);
  const [notice, setNotice] = useState<{ type: "error" | "success"; text: string } | null>(null);

  useEffect(() => () => {
    if (previewUrl) URL.revokeObjectURL(previewUrl);
  }, [previewUrl]);

  const reset = () => {
    setFile(null);
    setPhoto(null);
    setPreviewUrl("");
    setResult(null);
    setFeature(null);
    setNotice(null);
    setBbox(emptyBox);
    if (inputRef.current) inputRef.current.value = "";
  };

  const acceptFile = async (nextFile?: File) => {
    if (!nextFile) return;
    setNotice(null);
    setResult(null);
    setFeature(null);
    if (!['image/jpeg', 'image/png'].includes(nextFile.type)) {
      setNotice({ type: "error", text: "Поддерживаются только изображения JPEG и PNG." });
      return;
    }
    if (nextFile.size > 10 * 1024 * 1024) {
      setNotice({ type: "error", text: "Файл больше 10 МБ. Выберите изображение меньшего размера." });
      return;
    }

    if (previewUrl) URL.revokeObjectURL(previewUrl);
    const localUrl = URL.createObjectURL(nextFile);
    setFile(nextFile);
    setPreviewUrl(localUrl);
    setPhoto(null);
    setUploading(true);
    setImageId(nextFile.name);
    try {
      const uploaded = await api.uploadPhoto(nextFile);
      setPhoto(uploaded);
      setBbox({
        x: Math.round(uploaded.width * 0.12),
        y: Math.round(uploaded.height * 0.12),
        w: Math.max(1, Math.round(uploaded.width * 0.76)),
        h: Math.max(1, Math.round(uploaded.height * 0.76)),
      });
    } catch (error) {
      setNotice({ type: "error", text: errorMessage(error) });
    } finally {
      setUploading(false);
    }
  };

  const onDrop = (event: DragEvent) => {
    event.preventDefault();
    setDragActive(false);
    void acceptFile(event.dataTransfer.files[0]);
  };

  const setBoxField = (field: keyof BBox, raw: string) => {
    if (!photo) return;
    const value = Math.max(field === "w" || field === "h" ? 1 : 0, Number.parseInt(raw || "0", 10));
    const next = { ...bbox, [field]: value };
    next.x = Math.min(next.x, photo.width - 1);
    next.y = Math.min(next.y, photo.height - 1);
    next.w = Math.max(1, Math.min(next.w, photo.width - next.x));
    next.h = Math.max(1, Math.min(next.h, photo.height - next.y));
    setBbox(next);
  };

  const runSearch = async () => {
    if (!photo) return;
    setBusy("search");
    setResult(null);
    setNotice(null);
    try {
      const ids = excludeIds.split(/[\s,;]+/).map((id) => id.trim()).filter(Boolean);
      const found = await api.search({
        photo_id: photo.id,
        bbox,
        mode,
        top_k: topK,
        ...(mode === "candidates" ? { threshold } : {}),
        ...(ids.length ? { exclude_observation_ids: ids } : {}),
      });
      setResult(found);
      window.setTimeout(() => document.getElementById("search-results")?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
    } catch (error) {
      setNotice({ type: "error", text: errorMessage(error) });
    } finally {
      setBusy(null);
    }
  };

  const addToGallery = async () => {
    if (!photo) return;
    setBusy("gallery");
    setNotice(null);
    try {
      await api.addObservation({
        photo_id: photo.id,
        bbox,
        ...(imageId.trim() ? { image_id: imageId.trim() } : {}),
        ...(vehicleId.trim() ? { vehicle_id: vehicleId.trim() } : {}),
      });
      setNotice({ type: "success", text: "Объект добавлен в галерею и готов к поиску." });
      onGalleryChanged();
    } catch (error) {
      setNotice({ type: "error", text: errorMessage(error) });
    } finally {
      setBusy(null);
    }
  };

  const extractFeature = async () => {
    if (!photo) return;
    setBusy("feature");
    setNotice(null);
    try {
      setFeature(await api.extractFeature({ photo_id: photo.id, bbox }));
    } catch (error) {
      setNotice({ type: "error", text: errorMessage(error) });
    } finally {
      setBusy(null);
    }
  };

  const copyFeature = async () => {
    if (!feature) return;
    try {
      await navigator.clipboard.writeText(JSON.stringify(feature.embedding));
      setNotice({ type: "success", text: "Вектор из 512 значений скопирован." });
    } catch {
      setNotice({ type: "error", text: "Браузер не разрешил скопировать вектор." });
    }
  };

  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <div className="eyebrow"><Sparkles size={14} /> визуальный ReID</div>
          <h1>Найдите автомобиль<br />по одному кадру</h1>
          <p>Загрузите изображение, обведите нужный автомобиль и сравните его с объектами галереи.</p>
        </div>
        <div className="heading-status">
          <span className={`health-dot health-dot--${health}`} />
          <div><strong>{health === "online" ? "Система готова" : health === "checking" ? "Проверяем сервисы" : "Нет соединения"}</strong><span>ResNet50 · 512D embedding</span></div>
        </div>
      </div>

      {notice && (
        <div className={`notice notice--${notice.type}`} role="status">
          {notice.type === "success" ? <Check size={18} /> : <AlertCircle size={18} />}
          <span>{notice.text}</span>
          <button onClick={() => setNotice(null)} aria-label="Закрыть"><X size={16} /></button>
        </div>
      )}

      <section className="workflow-card">
        <div className="workflow-card__header">
          <div className="stepper">
            <div className="step step--active"><span>1</span><div><strong>Исходный кадр</strong><small>JPEG или PNG</small></div></div>
            <div className={`step-line ${photo ? "step-line--done" : ""}`} />
            <div className={`step ${photo ? "step--active" : ""}`}><span>2</span><div><strong>Область объекта</strong><small>точный BBox</small></div></div>
            <div className="step-line" />
            <div className={`step ${result ? "step--active" : ""}`}><span>3</span><div><strong>Совпадения</strong><small>косинусный поиск</small></div></div>
          </div>
          {file && <button className="ghost-button" type="button" onClick={reset}><RotateCcw size={15} /> Новый кадр</button>}
        </div>

        <div className="workflow-grid">
          <div className="workspace-panel">
            {!file ? (
              <div
                className={`dropzone ${dragActive ? "dropzone--active" : ""}`}
                onDragEnter={(event) => { event.preventDefault(); setDragActive(true); }}
                onDragOver={(event) => event.preventDefault()}
                onDragLeave={() => setDragActive(false)}
                onDrop={onDrop}
                onClick={() => inputRef.current?.click()}
                role="button"
                tabIndex={0}
                onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") inputRef.current?.click(); }}
              >
                <div className="dropzone__icon"><UploadCloud size={31} /></div>
                <h2>Перетащите сюда кадр</h2>
                <p>или нажмите, чтобы выбрать файл</p>
                <span>JPEG, PNG · до 10 МБ · до 40 Мп</span>
              </div>
            ) : uploading || !photo ? (
              <div className="upload-state">
                {uploading ? <LoaderCircle className="spin" size={34} /> : <AlertCircle size={34} />}
                <strong>{uploading ? "Загружаем исходный кадр…" : "Кадр не загружен"}</strong>
                <span>{file.name} · {formatBytes(file.size)}</span>
                {!uploading && <button className="secondary-button" onClick={() => void acceptFile(file)}>Повторить</button>}
              </div>
            ) : (
              <ImageAnnotator src={previewUrl} width={photo.width} height={photo.height} bbox={bbox} onChange={setBbox} />
            )}
            <input ref={inputRef} type="file" accept="image/jpeg,image/png" hidden onChange={(event: ChangeEvent<HTMLInputElement>) => void acceptFile(event.target.files?.[0])} />
          </div>

          <aside className="control-panel">
            <div className="control-panel__section">
              <div className="section-title"><div><span>01</span><strong>Координаты объекта</strong></div><CrosshairIcon /></div>
              <p className="helper-text">Растяните рамку мышью или задайте точные координаты.</p>
              <div className="coordinate-grid">
                {(["x", "y", "w", "h"] as const).map((field) => (
                  <label key={field} className="field field--compact"><span>{field.toUpperCase()}</span><input type="number" value={bbox[field]} min={field === "w" || field === "h" ? 1 : 0} disabled={!photo} onChange={(event) => setBoxField(field, event.target.value)} /><small>px</small></label>
                ))}
              </div>
            </div>

            <div className="control-panel__section">
              <div className="section-title"><div><span>02</span><strong>Параметры поиска</strong></div><SlidersHorizontal size={18} /></div>
              <div className="segmented" role="group" aria-label="Режим поиска">
                <button className={mode === "candidates" ? "segmented__active" : ""} onClick={() => setMode("candidates")}>Надёжные</button>
                <button className={mode === "ranking" ? "segmented__active" : ""} onClick={() => setMode("ranking")}>Топ похожих</button>
              </div>
              <div className="parameter-row">
                <label>Количество результатов</label>
                <input className="number-input" type="number" min={1} max={100} value={topK} onChange={(event) => setTopK(Math.max(1, Math.min(100, Number(event.target.value))))} />
              </div>
              {mode === "candidates" && (
                <div className="range-field">
                  <div><label>Порог сходства</label><strong>{threshold.toFixed(2)}</strong></div>
                  <input type="range" min={-1} max={1} step={0.01} value={threshold} onChange={(event) => setThreshold(Number(event.target.value))} />
                  <div className="range-field__labels"><span>−1</span><span>строже</span><span>1</span></div>
                </div>
              )}
              <button className="advanced-toggle" onClick={() => setAdvancedOpen((value) => !value)}>
                Расширенные параметры <ChevronDown className={advancedOpen ? "rotated" : ""} size={16} />
              </button>
              {advancedOpen && (
                <div className="advanced-panel">
                  <label className="field"><span>Исключить ID наблюдений</span><textarea rows={2} value={excludeIds} onChange={(event) => setExcludeIds(event.target.value)} placeholder="UUID через запятую" /></label>
                  <button className="feature-button" disabled={!photo || busy !== null || health === "offline"} onClick={extractFeature}>
                    {busy === "feature" ? <LoaderCircle className="spin" size={15} /> : <Braces size={15} />}
                    {busy === "feature" ? "Извлекаем признак…" : "Извлечь 512D-вектор"}
                  </button>
                  {feature && (
                    <div className="feature-result">
                      <div><span>{feature.model_version}</span><button onClick={copyFeature} title="Скопировать весь вектор"><Copy size={13} /></button></div>
                      <code>{feature.embedding.slice(0, 8).map((value) => value.toFixed(4)).join("  ")} …</code>
                      <small>{feature.embedding.length} значений · L2-нормированный</small>
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="control-panel__actions">
              <button className="primary-button" disabled={!photo || busy !== null || health === "offline"} onClick={runSearch}>
                {busy === "search" ? <LoaderCircle className="spin" size={18} /> : <Search size={18} />}
                {busy === "search" ? "Ищем совпадения…" : "Найти автомобиль"}
                {busy !== "search" && <ArrowRight size={18} />}
              </button>
              <div className="divider"><span>или сохранить образец</span></div>
              <div className="gallery-fields">
                <label className="field"><span>ID изображения</span><input value={imageId} onChange={(event) => setImageId(event.target.value)} placeholder="camera_01_00042.jpg" /></label>
                <label className="field"><span>ID автомобиля <em>необязательно</em></span><input value={vehicleId} onChange={(event) => setVehicleId(event.target.value)} placeholder="vehicle_042" /></label>
              </div>
              <button className="secondary-button secondary-button--wide" disabled={!photo || busy !== null || health === "offline"} onClick={addToGallery}>
                {busy === "gallery" ? <LoaderCircle className="spin" size={17} /> : <Plus size={17} />} Добавить в галерею
              </button>
            </div>
          </aside>
        </div>
      </section>

      {result && (
        <section className="results" id="search-results">
          <div className="results__heading">
            <div>
              <div className="eyebrow"><Search size={14} /> результат поиска</div>
              <h2>{result.refused ? "Уверенных совпадений нет" : `Найдено: ${result.candidates.length}`}</h2>
              <p>{result.refused ? (result.refusal_reason === "empty_gallery" ? "Галерея пуста — сначала добавьте хотя бы один объект." : "Ни один объект не прошёл заданный порог сходства.") : `Модель ${result.model_version} · результаты отсортированы по сходству`}</p>
            </div>
            <button className="secondary-button" onClick={onOpenGallery}><Database size={16} /> Открыть галерею</button>
          </div>

          {result.candidates.length ? (
            <div className="result-grid">
              {result.candidates.map((candidate, index) => <ObservationCard key={candidate.observation.id} observation={candidate.observation} confidence={candidate.confidence} rank={index + 1} />)}
            </div>
          ) : (
            <div className="empty-result"><div><ImagePlus size={27} /></div><strong>{result.refusal_reason === "empty_gallery" ? "В галерее пока нет образцов" : "Попробуйте снизить порог"}</strong><span>Измените параметры поиска или добавьте размеченный объект в галерею.</span></div>
          )}

          <div className="results__note"><Info size={15} /><span>Сходство — косинусная метрика, а не вероятность совпадения.</span></div>
        </section>
      )}
    </div>
  );
}

function CrosshairIcon() {
  return <div className="mini-crosshair"><span /><span /></div>;
}
