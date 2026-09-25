"use client";

import { useCallback, useEffect, useState } from "react";
import { Activity, Aperture, Database, FileJson, Menu, Search, X } from "lucide-react";
import { API_URL, api } from "@/lib/api";
import { SearchWorkspace } from "./search-workspace";
import { GalleryView } from "./gallery-view";

type View = "search" | "gallery";
type Health = "checking" | "online" | "offline";

export function ReIdApp() {
  const [view, setView] = useState<View>("search");
  const [health, setHealth] = useState<Health>("checking");
  const [menuOpen, setMenuOpen] = useState(false);
  const [galleryRevision, setGalleryRevision] = useState(0);

  const checkHealth = useCallback(async () => {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 4500);
    try {
      await api.readiness(controller.signal);
      setHealth("online");
    } catch {
      setHealth("offline");
    } finally {
      window.clearTimeout(timeout);
    }
  }, []);

  useEffect(() => {
    void checkHealth();
    const timer = window.setInterval(checkHealth, 30000);
    return () => window.clearInterval(timer);
  }, [checkHealth]);

  const navigate = (next: View) => {
    setView(next);
    setMenuOpen(false);
  };

  return (
    <div className="app-shell">
      <aside className={`sidebar ${menuOpen ? "sidebar--open" : ""}`}>
        <div className="brand">
          <div className="brand__mark"><Aperture size={22} strokeWidth={2.4} /></div>
          <div>
            <strong>VECTOR.ID</strong>
            <span>vehicle intelligence</span>
          </div>
        </div>

        <button className="sidebar__close" onClick={() => setMenuOpen(false)} aria-label="Закрыть меню">
          <X size={21} />
        </button>

        <nav className="nav" aria-label="Основная навигация">
          <span className="nav__label">Рабочее пространство</span>
          <button className={view === "search" ? "nav__item nav__item--active" : "nav__item"} onClick={() => navigate("search")}>
            <Search size={19} />
            <span>Поиск совпадений</span>
          </button>
          <button className={view === "gallery" ? "nav__item nav__item--active" : "nav__item"} onClick={() => navigate("gallery")}>
            <Database size={19} />
            <span>Галерея объектов</span>
          </button>
        </nav>

        <div className="sidebar__spacer" />

        <div className="system-card">
          <div className="system-card__heading">
            <Activity size={17} />
            <span>Состояние системы</span>
          </div>
          <button className="health-row" onClick={checkHealth} title="Проверить снова">
            <span className={`health-dot health-dot--${health}`} />
            <span>{health === "online" ? "Все сервисы готовы" : health === "offline" ? "Сервис недоступен" : "Проверяем…"}</span>
          </button>
          <div className="system-card__meta">API · {API_URL.replace(/^https?:\/\//, "")}</div>
        </div>

        <a className="sidebar__footer" href={`${API_URL}/openapi.json`} target="_blank" rel="noreferrer">
          <FileJson size={16} /> Контракт API <span>↗</span>
        </a>
      </aside>

      {menuOpen && <button className="sidebar-scrim" onClick={() => setMenuOpen(false)} aria-label="Закрыть меню" />}

      <main className="main">
        <header className="mobile-header">
          <button className="icon-button" onClick={() => setMenuOpen(true)} aria-label="Открыть меню"><Menu size={22} /></button>
          <div className="brand brand--mobile">
            <div className="brand__mark"><Aperture size={19} /></div>
            <strong>VECTOR.ID</strong>
          </div>
          <span className={`health-dot health-dot--${health}`} />
        </header>

        {view === "search" ? (
          <SearchWorkspace
            health={health}
            onGalleryChanged={() => setGalleryRevision((value) => value + 1)}
            onOpenGallery={() => navigate("gallery")}
          />
        ) : (
          <GalleryView key={galleryRevision} onBackToSearch={() => navigate("search")} />
        )}
      </main>
    </div>
  );
}
