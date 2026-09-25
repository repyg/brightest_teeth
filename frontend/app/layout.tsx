import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Vector ID — поиск автомобилей",
  description: "Интерфейс сервиса визуального сопоставления автомобилей",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru">
      <body>{children}</body>
    </html>
  );
}
