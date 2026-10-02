"use client";

/* eslint-disable @next/next/no-img-element */

import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  BarChart3,
  Clock3,
  Database,
  ExternalLink,
  FileImage,
  HardDrive,
  LoaderCircle,
  RefreshCw,
  Settings,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import { API_URL, MLFLOW_URL, api, photoContentUrl } from "@/lib/api";
import { errorMessage, formatDate, shortId } from "@/lib/format";
import type { AdminStats, Observation } from "@/lib/types";

type Status = "checking" | "online" | "offline";
type Props = { onOpenGallery: () => void };

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} Б`;
  const units = ["КБ", "МБ", "ГБ", "ТБ"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit += 1; }
  return `${value.toLocaleString("ru-RU", { maximumFractionDigits: value >= 10 ? 1 : 2 })} ${units[unit]}`;
}

export function AdminView({ onOpenGallery }: Props) {
  const [stats, setStats] = useState<AdminStats | null>(null);
  const [observations, setObservations] = useState<Observation[]>([]);
  const [system, setSystem] = useState<Status>("checking");
  const [mlflow, setMlflow] = useState<Status>("checking");
  const [loading, setLoading] = useState(true);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    setSystem("checking");
    setMlflow("checking");
    const [statsResult, pageResult, readyResult, mlflowResult] = await Promise.allSettled([
      api.adminStats(),
      api.listObservations(undefined, 8),
      api.readiness(),
      fetch(`${MLFLOW_URL}/health`, { cache: "no-store" }).then((response) => {
        if (!response.ok) throw new Error("MLflow unavailable");
      }),
    ]);
    if (statsResult.status === "fulfilled") setStats(statsResult.value);
    if (pageResult.status === "fulfilled") setObservations(pageResult.value.items);
    setSystem(readyResult.status === "fulfilled" ? "online" : "offline");
    setMlflow(mlflowResult.status === "fulfilled" ? "online" : "offline");
    const failure = [statsResult, pageResult].find((result) => result.status === "rejected");
    if (failure?.status === "rejected") setError(errorMessage(failure.reason));
    setLoading(false);
  }, []);

  useEffect(() => { void load(); }, [load]);

  const remove = async (observation: Observation) => {
    if (!window.confirm(`Удалить наблюдение ${shortId(observation.id)} из галереи?`)) return;
    setDeleting(observation.id);
    setError("");
    try {
      await api.deleteObservation(observation.id);
      await load();
    } catch (deleteError) {
      setError(errorMessage(deleteError));
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="page admin-page">
      <div className="page-heading page-heading--gallery">
        <div>
          <div className="eyebrow"><Settings size={14} /> управление системой</div>
          <h1>Панель администратора</h1>
          <p>Состояние компонентов, статистика хранилища и последние объекты галереи.</p>
        </div>
        <button className="secondary-button admin-refresh" onClick={() => void load()} disabled={loading}>
          <RefreshCw className={loading ? "spin" : ""} size={16} /> Обновить
        </button>
      </div>

      {error && <div className="notice notice--error"><X size={18} /><span>{error}</span><button onClick={() => setError("")} aria-label="Закрыть"><X size={16} /></button></div>}

      <section className="admin-metrics" aria-label="Статистика">
        <article><span><Database size={20} /></span><div><strong>{stats?.observations_count.toLocaleString("ru-RU") ?? "—"}</strong><small>наблюдений в галерее</small></div></article>
        <article><span><FileImage size={20} /></span><div><strong>{stats?.photos_count.toLocaleString("ru-RU") ?? "—"}</strong><small>загруженных кадров</small></div></article>
        <article><span><HardDrive size={20} /></span><div><strong>{stats ? formatBytes(stats.storage_bytes) : "—"}</strong><small>зарегистрировано в MinIO</small></div></article>
        <article><span><Clock3 size={20} /></span><div><strong>{stats?.last_observation_at ? formatDate(stats.last_observation_at) : "Нет данных"}</strong><small>последнее наблюдение</small></div></article>
      </section>

      <div className="admin-grid">
        <section className="admin-panel">
          <div className="admin-panel__heading"><div><Activity size={18} /><div><strong>Компоненты</strong><span>Готовность инфраструктуры</span></div></div></div>
          <div className="service-list">
            <div><span className={`health-dot health-dot--${system}`} /><div><strong>Vehicle ReID API</strong><small>{API_URL}</small></div><b>{system === "online" ? "Готов" : system === "offline" ? "Недоступен" : "Проверка"}</b></div>
            <div><span className={`health-dot health-dot--${system}`} /><div><strong>PostgreSQL · MinIO · ML worker</strong><small>Агрегированная проверка /readyz</small></div><b>{system === "online" ? "Готовы" : system === "offline" ? "Ошибка" : "Проверка"}</b></div>
            <div><span className={`health-dot health-dot--${mlflow}`} /><div><strong>MLflow Tracking</strong><small>{MLFLOW_URL}</small></div><b>{mlflow === "online" ? "Готов" : mlflow === "offline" ? "Недоступен" : "Проверка"}</b></div>
          </div>
        </section>

        <section className="admin-panel admin-panel--mlflow">
          <div className="mlflow-mark"><BarChart3 size={25} /></div>
          <div><span className="eyebrow">эксперименты и метрики</span><h2>MLflow</h2><p>Сравнивайте запуски обучения, loss, порог отказа и артефакты модели.</p></div>
          <a className="primary-button primary-button--compact" href={MLFLOW_URL} target="_blank" rel="noreferrer">Открыть MLflow <ExternalLink size={16} /></a>
        </section>
      </div>

      <section className="admin-panel admin-recent">
        <div className="admin-panel__heading">
          <div><ShieldCheck size={18} /><div><strong>Последние наблюдения</strong><span>Быстрое управление галереей</span></div></div>
          <button className="ghost-button" onClick={onOpenGallery}>Вся галерея</button>
        </div>
        {loading && !stats ? <div className="admin-loading"><LoaderCircle className="spin" size={24} /> Загружаем данные…</div> : observations.length ? (
          <div className="admin-table-wrap"><table className="admin-table"><thead><tr><th>Кадр</th><th>Наблюдение</th><th>Автомобиль</th><th>Модель</th><th>Добавлено</th><th /></tr></thead><tbody>
            {observations.map((observation) => <tr key={observation.id}>
              <td><a href={photoContentUrl(observation.photo_id)} target="_blank" rel="noreferrer" className="admin-thumb"><img src={photoContentUrl(observation.photo_id)} alt="" /></a></td>
              <td><strong>{shortId(observation.id)}</strong><small>{observation.image_id || shortId(observation.photo_id)}</small></td>
              <td>{observation.vehicle_id || "—"}</td><td><code>{observation.model_version}</code></td><td>{formatDate(observation.created_at)}</td>
              <td><button className="icon-button icon-button--danger" onClick={() => void remove(observation)} disabled={deleting === observation.id} aria-label="Удалить">{deleting === observation.id ? <LoaderCircle className="spin" size={16} /> : <Trash2 size={16} />}</button></td>
            </tr>)}
          </tbody></table></div>
        ) : <div className="admin-loading">Галерея пока пуста.</div>}
      </section>
    </div>
  );
}
