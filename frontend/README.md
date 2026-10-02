# Vehicle ReID Console

Next.js-интерфейс для API сопоставления автомобилей. Покрывает загрузку кадров,
интерактивную разметку BBox, поиск, добавление в галерею, просмотр и удаление
наблюдений. Раздел «Администрирование» показывает состояние компонентов,
агрегаты PostgreSQL/MinIO, последние наблюдения и ссылку на MLflow.

```bash
npm install
npm run dev
```

По умолчанию API ожидается на `http://localhost:8080`, MLflow — на
`http://localhost:5000`. Адреса задаются через `NEXT_PUBLIC_API_URL` и
`NEXT_PUBLIC_MLFLOW_URL`. Это публичные build-time переменные Next.js.
