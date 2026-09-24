import os
import torch
import pandas as pd
import numpy as np
from torch.utils.data import Dataset, DataLoader
from PIL import Image
from tqdm import tqdm

# Импортируем вашу модель и трансформации
from core.model import VehicleReIDModel
from expeiments.dataset import test_transform

# --- 1. Настройка локальных путей ---
BASE_DIR = os.path.dirname(os.path.abspath(__file__))

# Пути к исходным CSV с разметкой
DATASET_ROOT = os.path.join(BASE_DIR, "../Data", "dataset")
QUERY_CSV = os.path.join(DATASET_ROOT, "test_query.csv")
GALLERY_CSV = os.path.join(DATASET_ROOT, "test_gallery.csv")

# Пути к нарезанным картинкам
QUERY_DIR = os.path.join(BASE_DIR, "../crops", "query")
GALLERY_DIR = os.path.join(BASE_DIR, "../crops", "bounding_box_test")

# Путь к весам модели и папке для сохранения результатов
WEIGHTS_PATH = os.path.join(BASE_DIR, "../core/weights", "reid_model_final.pth")
OUTPUT_DIR = BASE_DIR  # Сохраним результаты прямо в корень проекта


# --- 2. Датасет для инференса ---
class InferDataset(Dataset):
    def __init__(self, csv_file, img_dir, transform):
        self.df = pd.read_csv(csv_file)
        self.img_dir = img_dir
        self.transform = transform

    def __len__(self):
        return len(self.df)

    def __getitem__(self, idx):
        # Получаем ID изображения по порядку из CSV
        img_id = str(self.df.iloc[idx]['image_id'])
        img_name = img_id if img_id.endswith(('.jpg', '.png')) else f"{img_id}.jpg"
        img_path = os.path.join(self.img_dir, img_name)

        # PIL отлично справляется с путями на кириллице в Windows
        image = Image.open(img_path).convert('RGB')
        image = np.array(image)

        if self.transform:
            image = self.transform(image=image)['image']

        return image, img_id


# --- 3. Функция извлечения векторов ---
def extract_features(loader, model, device):
    model.eval()
    features = []
    img_ids = []

    with torch.no_grad():
        for images, ids in tqdm(loader, desc="Извлечение признаков"):
            images = images.to(device)
            # При инференсе наша модель (в model.py) уже возвращает нормализованные векторы
            embeds = model(images)
            features.append(embeds.cpu())
            img_ids.extend(ids)

    return torch.cat(features, dim=0), img_ids


# --- 4. Основной процесс ---
def run_inference():
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    print(f"Запуск инференса на устройстве: {device}")

    # Проверка наличия весов
    if not os.path.exists(WEIGHTS_PATH):
        raise FileNotFoundError(
            f"Файл с весами не найден по пути: {WEIGHTS_PATH}. Создайте папку weights и положите туда модель.")

    # Инициализация и загрузка весов
    # num_classes = 1541 — число, на котором мы обучались (важно для инициализации)
    model = VehicleReIDModel(num_classes=1541, model_name='resnet50', embedding_size=512)
    model.load_state_dict(torch.load(WEIGHTS_PATH, map_location=device))
    model.to(device)

    # Даталоадеры (shuffle=False ОБЯЗАТЕЛЕН, чтобы порядок векторов совпал с CSV)
    query_dataset = InferDataset(QUERY_CSV, QUERY_DIR, test_transform)
    gallery_dataset = InferDataset(GALLERY_CSV, GALLERY_DIR, test_transform)

    # num_workers=0 для Windows, чтобы избежать зависаний при инференсе
    query_loader = DataLoader(query_dataset, batch_size=64, shuffle=False, num_workers=0)
    gallery_loader = DataLoader(gallery_dataset, batch_size=64, shuffle=False, num_workers=0)

    # Извлечение эмбеддингов
    print("\nОбработка запросов (Query)...")
    q_feats, q_ids = extract_features(query_loader, model, device)

    print("\nОбработка галереи (Gallery)...")
    g_feats, g_ids = extract_features(gallery_loader, model, device)

    # ---------------------------------------------------------
    # АРТЕФАКТ 1: embeddings.npy (Сначала запросы, потом галерея)
    # ---------------------------------------------------------
    all_embeddings = torch.cat([q_feats, g_feats], dim=0).numpy()
    np.save(os.path.join(OUTPUT_DIR, "../embeddings.npy"), all_embeddings)
    print(f"\nСохранен embeddings.npy (Размер матрицы: {all_embeddings.shape})")

    # ---------------------------------------------------------
    # АРТЕФАКТ 2: submission.csv (Матричное умножение сходства)
    # ---------------------------------------------------------
    # Умножаем матрицу запросов на транспонированную матрицу галереи
    sim_matrix = torch.mm(q_feats, g_feats.T)

    submission_data = []
    candidates_data = ["query_id,gallery_id,confidence\n"]  # Заголовок для файла отказов

    # Порог уверенности для режима отказа (настраиваемый параметр)
    CONFIDENCE_THRESHOLD = 0.36

    print("Формирование submission.csv и candidates.csv...")
    for i in range(len(q_ids)):
        q_id = q_ids[i]
        scores = sim_matrix[i].numpy()

        # Находим индексы топ-10 кандидатов по убыванию сходства
        top10_idx = np.argsort(-scores)[:10]
        top10_g_ids = [g_ids[idx] for idx in top10_idx]

        # Строка формата: query_id, gallery_id_1, ..., gallery_id_10
        submission_data.append([q_id] + top10_g_ids)

        # ---------------------------------------------------------
        # АРТЕФАКТ 3: candidates.csv (Режим отказа)
        # ---------------------------------------------------------
        best_score = scores[top10_idx[0]]
        # Если уверенность самой похожей машины выше порога — записываем
        if best_score >= CONFIDENCE_THRESHOLD:
            candidates_data.append(f"{q_id},{top10_g_ids[0]},{best_score:.4f}\n")
        # Если ниже порога — ничего не пишем (система оценки засчитает отказ)

    # Запись submission.csv
    import csv
    with open(os.path.join(OUTPUT_DIR, "../submission.csv"), "w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerows(submission_data)

    # Запись candidates.csv
    with open(os.path.join(OUTPUT_DIR, "../candidates.csv"), "w", encoding="utf-8") as f:
        f.writelines(candidates_data)

    print(f"Готово! Все три файла успешно сгенерированы в папке: {OUTPUT_DIR}")


if __name__ == "__main__":
    run_inference()