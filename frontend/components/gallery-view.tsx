"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Database,
  Inbox,
  LoaderCircle,
  RefreshCw,
  Search,
  ShieldCheck,
  X,
} from "lucide-react";
import { api } from "@/lib/api";
import { errorMessage, shortId } from "@/lib/format";
import type { Observation, ObservationPage } from "@/lib/types";
import { ObservationCard } from "./observation-card";

type Props = { onBackToSearch: () => void };

export function GalleryView({ onBackToSearch }: Props) {
  const [page, setPage] = useState<ObservationPage | null>(null);
  const [cursorStack, setCursorStack] = useState<Array<string | undefined>>([undefined]);
  const [pageIndex, setPageIndex] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Observation | null>(null);
  const [query, setQuery] = useState("");

  const load = useCallback(async (cursor?: string) => {
    setLoading(true);
    setError("");
    try {
      setPage(await api.listObservations(cursor));
    } catch (loadError) {
      setError(errorMessage(loadError));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const nextPage = () => {
    if (!page?.next_cursor) return;
    const nextStack = cursorStack.slice(0, pageIndex + 1);
    nextStack.push(page.next_cursor);
    setCursorStack(nextStack);
    setPageIndex(pageIndex + 1);
    void load(page.next_cursor);
  };

  const previousPage = () => {
    if (pageIndex === 0) return;
    const nextIndex = pageIndex - 1;
    setPageIndex(nextIndex);
    void load(cursorStack[nextIndex]);
  };

  const remove = async () => {
    if (!pendingDelete) return;
    setDeleting(pendingDelete.id);
    try {
      await api.deleteObservation(pendingDelete.id);
      setPendingDelete(null);
      await load(cursorStack[pageIndex]);
    } catch (deleteError) {
      setError(errorMessage(deleteError));
      setPendingDelete(null);
    } finally {
      setDeleting(null);
    }
  };

  const normalizedQuery = query.trim().toLowerCase();
  const visible = (page?.items || []).filter((item) => !normalizedQuery || [item.id, item.photo_id, item.image_id, item.vehicle_id].some((value) => value?.toLowerCase().includes(normalizedQuery)));

  return (
    <div className="page gallery-page">
      <div className="page-heading page-heading--gallery">
        <div>
          <div className="eyebrow"><Database size={14} /> база наблюдений</div>
          <h1>Галерея объектов</h1>
          <p>Образцы автомобилей, по которым выполняется визуальный поиск.</p>
        </div>
        <button className="primary-button primary-button--compact" onClick={onBackToSearch}><Search size={17} /> Новый поиск</button>
      </div>

      <div className="gallery-toolbar">
        <label className="search-field"><Search size={17} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="ID объекта, кадра или автомобиля" />{query && <button onClick={() => setQuery("")} aria-label="Очистить"><X size={15} /></button>}</label>
        <div className="gallery-toolbar__right">
          <div className="page-count"><span>Страница</span><strong>{pageIndex + 1}</strong></div>
          <button className="icon-button" onClick={() => void load(cursorStack[pageIndex])} disabled={loading} aria-label="Обновить"><RefreshCw className={loading ? "spin" : ""} size={18} /></button>
        </div>
      </div>

      {error && <div className="notice notice--error"><X size={18} /><span>{error}</span><button onClick={() => setError("")} aria-label="Закрыть"><X size={16} /></button></div>}

      <div className="gallery-summary">
        <div><span className="gallery-summary__icon"><Database size={20} /></span><div><strong>{page?.items.length ?? "—"}</strong><span>объектов на странице</span></div></div>
        <div><span className="gallery-summary__icon gallery-summary__icon--green"><ShieldCheck size={20} /></span><div><strong>{page?.items[0]?.model_version || "reid-resnet50-v1"}</strong><span>активная модель</span></div></div>
      </div>

      {loading && !page ? (
        <div className="gallery-loading"><LoaderCircle className="spin" size={32} /><strong>Загружаем галерею…</strong></div>
      ) : visible.length ? (
        <div className={`gallery-grid ${loading ? "gallery-grid--loading" : ""}`}>
          {visible.map((observation) => <ObservationCard key={observation.id} observation={observation} deleting={deleting === observation.id} onDelete={setPendingDelete} />)}
        </div>
      ) : (
        <div className="gallery-empty">
          <div><Inbox size={30} /></div>
          <h2>{query ? "Ничего не найдено" : "Галерея пока пуста"}</h2>
          <p>{query ? "Измените строку поиска — фильтрация действует на текущей странице." : "Загрузите кадр, выделите автомобиль и добавьте его как образец."}</p>
          {!query && <button className="primary-button primary-button--compact" onClick={onBackToSearch}>Добавить первый объект <ArrowRight size={17} /></button>}
        </div>
      )}

      {page && (pageIndex > 0 || page.next_cursor) && (
        <div className="pagination">
          <button className="secondary-button" disabled={pageIndex === 0 || loading} onClick={previousPage}><ArrowLeft size={16} /> Назад</button>
          <span>Страница {pageIndex + 1}</span>
          <button className="secondary-button" disabled={!page.next_cursor || loading} onClick={nextPage}>Дальше <ArrowRight size={16} /></button>
        </div>
      )}

      {pendingDelete && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setPendingDelete(null)}>
          <div className="confirm-modal" role="dialog" aria-modal="true" aria-labelledby="delete-title" onMouseDown={(event) => event.stopPropagation()}>
            <button className="confirm-modal__close" onClick={() => setPendingDelete(null)} aria-label="Закрыть"><X size={19} /></button>
            <div className="confirm-modal__icon"><Database size={23} /></div>
            <h2 id="delete-title">Удалить объект из галереи?</h2>
            <p>Наблюдение <strong>{shortId(pendingDelete.id)}</strong> больше не будет участвовать в поиске. Исходный кадр сохранится.</p>
            <div className="confirm-modal__actions">
              <button className="secondary-button" onClick={() => setPendingDelete(null)}>Отмена</button>
              <button className="danger-button" onClick={remove} disabled={deleting !== null}>{deleting ? <LoaderCircle className="spin" size={16} /> : null}Удалить</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
