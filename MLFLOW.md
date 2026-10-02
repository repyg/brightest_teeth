# MLflow

MLflow запускается вместе с проектом и хранит метаданные запусков в SQLite,
а артефакты — в Docker volume `mlflow_data`.

```sh
docker compose up -d mlflow
```

Интерфейс: `http://localhost:5000`. Проверка состояния: `GET /health`.
Сбросить локальную историю можно только явно, удалив volume:
`docker compose down -v`.

## Эксперименты

Скрипты обучения, подбора порога и инференса автоматически создают запуск в
эксперименте `vehicle-reid` и сохраняют параметры, метрики и выходные файлы.
Перед локальным запуском установите зависимости и укажите tracking server:

```powershell
$env:MLFLOW_TRACKING_URI = "http://localhost:5000"
$env:MLFLOW_EXPERIMENT_NAME = "vehicle-reid"
python -m expeiments.train
```

Аналогично запускаются `python -m expeiments.find_threshold` и
`python -m expeiments.inference`. Имя запуска можно задать переменной
`MLFLOW_RUN_NAME`. Для отключения внешнего сервера не задавайте URI: по
умолчанию скрипты используют локальный `http://localhost:5000`.

Основные переменные находятся в `.env.example`:

| Переменная | Назначение |
| --- | --- |
| `MLFLOW_PORT` | Внешний порт MLflow |
| `MLFLOW_TRACKING_URI` | URI tracking server для Python-скриптов |
| `MLFLOW_EXPERIMENT_NAME` | Имя эксперимента |
| `MLFLOW_ALLOWED_HOSTS` | Разрешённые HTTP Host для tracking server |
| `MLFLOW_CORS_ALLOWED_ORIGINS` | Разрешённые origin для браузера |
| `NEXT_PUBLIC_MLFLOW_URL` | Ссылка из интерфейса администратора |

SQLite подходит для локальной разработки и небольшой команды. Для общего
стенда следует вынести backend store в PostgreSQL и артефакты в S3/MinIO,
а также ограничить разрешённые hosts/origins.
