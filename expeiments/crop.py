import os
import cv2
import pandas as pd
from tqdm import tqdm


def crop_and_save(csv_path, images_dir, output_dir, is_train=False):
    # Проверка существования файла аннотаций
    if not os.path.exists(csv_path):
        print(f"Файл не найден: {csv_path}")
        return

    df = pd.read_csv(csv_path)
    os.makedirs(output_dir, exist_ok=True)

    # Счетчик для генерации последовательности (seq) в формате Market-1501
    seq_counters = {}

    for idx, row in tqdm(df.iterrows(), total=len(df), desc=f"Обработка {os.path.basename(csv_path)}"):
        image_id = str(row['image_id'])

        # Защита на случай отсутствия расширения файла в csv
        img_name = image_id if image_id.lower().endswith(('.jpg', '.png', '.jpeg')) else f"{image_id}.jpg"
        img_path = os.path.join(images_dir, img_name)

        img = cv2.imread(img_path)
        if img is None:
            print(f"Ошибка загрузки: {img_path}")
            continue

        # Чтение координат BBox
        x, y = int(row['x']), int(row['y'])
        w, h = int(row['w']), int(row['h'])

        # Защита от выхода координат за границы изображения
        y_max, x_max = img.shape[:2]
        x1, y1 = max(0, x), max(0, y)
        x2, y2 = min(x_max, x + w), min(y_max, y + h)

        crop = img[y1:y2, x1:x2]
        if crop.size == 0:
            print(f"Пустой кроп для {img_name}")
            continue

        if is_train:
            # Формирование имени Market-1501: ID_c1_seq.jpg
            vid = str(row['vehicle_id']).zfill(4)
            seq = seq_counters.get(vid, 0) + 1
            seq_counters[vid] = seq
            save_name = f"{vid}_c1_{seq:04d}.jpg"
        else:
            # Для тестовой выборки сохраняем под оригинальным именем
            save_name = img_name

        cv2.imwrite(os.path.join(output_dir, save_name), crop)


if __name__ == "__main__":
    # Указываем правильный путь с учетом папки "Датасет"
    DATASET_ROOT = os.path.join("../Data", "dataset")
    IMAGES_DIR = os.path.join(DATASET_ROOT, "images")

    # Папка для сохранения результатов появится рядом с main.py
    OUTPUT_ROOT = "crops"

    train_csv = os.path.join(DATASET_ROOT, "train.csv")
    query_csv = os.path.join(DATASET_ROOT, "test_query.csv")
    gallery_csv = os.path.join(DATASET_ROOT, "test_gallery.csv")

    # Выполнение обрезки для всех трех выборок
    crop_and_save(train_csv, IMAGES_DIR, os.path.join(OUTPUT_ROOT, "bounding_box_train"), is_train=True)
    crop_and_save(query_csv, IMAGES_DIR, os.path.join(OUTPUT_ROOT, "query"), is_train=False)
    crop_and_save(gallery_csv, IMAGES_DIR, os.path.join(OUTPUT_ROOT, "bounding_box_test"), is_train=False)