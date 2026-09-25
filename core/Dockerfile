FROM python:3.11-slim
ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1 HF_HUB_OFFLINE=1 ALBUMENTATIONS_OFFLINE=1 NO_ALBUMENTATIONS_UPDATE=1
WORKDIR /app
COPY core/requirements-serving.txt /app/requirements-serving.txt
RUN pip install -r requirements-serving.txt
COPY core /app/core
USER 65532:65532
EXPOSE 8000
CMD ["python", "-m", "core.server"]
