import os
import sys
import glob
import torch
import numpy as np
import pandas as pd
import matplotlib.pyplot as plt
from PIL import Image
from tqdm import tqdm
from torch.utils.data import Dataset, DataLoader

# Добавляем корневую папку в sys.path для корректных импортов из соседних директорий
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
sys.path.append(BASE_DIR)

# Импортируем модель из папки core
from core.model import VehicleReIDModel
# Импортируем трансформации из вашей папки expeiments (с учетом опечатки в названии)
from expeiments.dataset import test_transform

# --- Настройки путей согласно структуре проекта ---
TRAIN_DIR = os.path.join(BASE_DIR, "../crops", "bounding_box_train")
WEIGHTS_PATH = os.path.join(BASE_DIR, "../core", "weights", "reid_model_final.pth")


class ThresholdDataset(Dataset):
    def __init__(self, img_dir, transform, max_samples=2000):
        # Собираем пути ко всем тренировочным картинкам
        all_paths = glob.glob(os.path.join(img_dir, "*.jpg"))

        # Для скорости берем случайные 2000 картинок (этого с запасом хватит для статистики)
        np.random.seed(42)
        if len(all_paths) > max_samples:
            self.img_paths = np.random.choice(all_paths, max_samples, replace=False)
        else:
            self.img_paths = all_paths

        self.transform = transform

    def __len__(self):
        return len(self.img_paths)

    def __getitem__(self, idx):
        img_path = self.img_paths[idx]

        # В вашем dataset.py файлы сохранялись как ID_c1_seq.jpg
        # Вытаскиваем ID автомобиля из начала имени файла
        basename = os.path.basename(img_path)
        vehicle_id = int(basename.split('_')[0])

        image = Image.open(img_path).convert('RGB')
        image = np.array(image)
        if self.transform:
            image = self.transform(image=image)['image']

        return image, vehicle_id


@torch.no_grad()
def main():
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    print(f"Анализ порогов запущен на устройстве: {device}")

    # Проверка наличия весов
    if not os.path.exists(WEIGHTS_PATH):
        raise FileNotFoundError(f"Файл с весами не найден по пути: {WEIGHTS_PATH}")

    # Загрузка модели (1541 класс, как было при обучении)
    model = VehicleReIDModel(num_classes=1541, model_name='resnet50', embedding_size=512)
    model.load_state_dict(torch.load(WEIGHTS_PATH, map_location=device))
    model.to(device)
    model.eval()

    # Загрузка данных (num_workers=0 для стабильности на Windows)
    dataset = ThresholdDataset(TRAIN_DIR, test_transform, max_samples=2000)
    loader = DataLoader(dataset, batch_size=64, shuffle=False, num_workers=0)

    # 1. Извлечение признаков
    all_embeds = []
    all_vids = []
    for images, vids in tqdm(loader, desc="Извлечение признаков для анализа"):
        # При инференсе модель возвращает нормализованные векторы
        embeds = model(images.to(device))
        all_embeds.append(embeds.cpu())
        all_vids.extend(vids.numpy())

    embeddings = torch.cat(all_embeds, dim=0)  # [N, 512]
    vids = np.array(all_vids)

    # 2. Вычисление матрицы косинусного сходства
    print("Вычисление попарного сходства...")
    sim_matrix = torch.mm(embeddings, embeddings.T).numpy()

    # 3. Разделение на позитивные (одна машина) и негативные (разные машины) пары
    labels_matrix = (vids[:, None] == vids[None, :])

    # Убираем главную диагональ (сравнение картинки с самой собой всегда = 1.0)
    mask = ~np.eye(len(vids), dtype=bool)

    sim_scores = sim_matrix[mask]
    true_labels = labels_matrix[mask]

    pos_scores = sim_scores[true_labels]
    neg_scores = sim_scores[~true_labels]

    print(f"Найдено позитивных пар (совпадений): {len(pos_scores)}")
    print(f"Найдено негативных пар (разные ТС): {len(neg_scores)}")

    # 4. Поиск идеального порога (максимизация F1-score)
    thresholds = np.linspace(0.3, 0.9, 100)
    best_f1 = 0
    best_thresh = 0

    for t in thresholds:
        tp = np.sum(pos_scores >= t)
        fp = np.sum(neg_scores >= t)
        fn = np.sum(pos_scores < t)

        precision = tp / (tp + fp) if (tp + fp) > 0 else 0
        recall = tp / (tp + fn) if (tp + fn) > 0 else 0

        if precision + recall > 0:
            f1 = 2 * precision * recall / (precision + recall)
            if f1 > best_f1:
                best_f1 = f1
                best_thresh = t

    print(f"\n--- РЕЗУЛЬТАТЫ ---")
    print(f"Оптимальный порог: {best_thresh:.3f}")
    print(f"Максимальный F1-score: {best_f1:.4f}")

    # 5. Отрисовка красивого графика для защиты и презентации
    plt.figure(figsize=(10, 6))

    # Строим нормализованные гистограммы
    plt.hist(neg_scores, bins=50, alpha=0.6, density=True, color='red', label='Разные машины (Negative)')
    plt.hist(pos_scores, bins=50, alpha=0.6, density=True, color='green', label='Одна машина (Positive)')

    # Рисуем линию нашего идеального порога
    plt.axvline(best_thresh, color='blue', linestyle='dashed', linewidth=2,
                label=f'Оптимальный порог: {best_thresh:.2f}')

    plt.title('Распределение косинусного сходства (Cosine Similarity)')
    plt.xlabel('Сходство (0.0 - 1.0)')
    plt.ylabel('Плотность пар')
    plt.legend(loc='upper right')
    plt.grid(True, alpha=0.3)

    # Сохраняем график в корне проекта
    plot_path = os.path.join(BASE_DIR, "threshold_plot.png")
    plt.savefig(plot_path, dpi=300, bbox_inches='tight')
    print(f"\nГрафик успешно сохранен в файл: {plot_path}")
    print(f"ОБЯЗАТЕЛЬНО: обновите CONFIDENCE_THRESHOLD в inference.py на {best_thresh:.2f}")


if __name__ == "__main__":
    main()