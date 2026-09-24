import os
import cv2
import torch
import numpy as np
from torch.utils.data import Dataset, DataLoader
import albumentations as A
from albumentations.pytorch import ToTensorV2


class VehicleReIDDataset(Dataset):
    def __init__(self, image_dir, transform=None):
        self.image_dir = image_dir
        self.transform = transform

        # Получаем список всех изображений в папке
        self.image_names = [f for f in os.listdir(image_dir) if f.endswith(('.jpg', '.png'))]

        # Собираем уникальные ID для маппинга в индексы от 0 до N
        # В названии 0042_c1_0001.jpg ID машины — это '0042'
        all_ids = sorted(list(set([name.split('_')[0] for name in self.image_names])))
        self.id_to_idx = {vid: idx for idx, vid in enumerate(all_ids)}
        self.num_classes = len(all_ids)

    def __len__(self):
        return len(self.image_names)

    def __getitem__(self, idx):
        img_name = self.image_names[idx]
        img_path = os.path.join(self.image_dir, img_name)

        # Читаем изображение (решение проблемы с кириллицей)
        stream = open(img_path, "rb")
        bytes_arr = bytearray(stream.read())
        np_arr = np.asarray(bytes_arr, dtype=np.uint8)
        image = cv2.imdecode(np_arr, cv2.IMREAD_COLOR)

        # Переводим BGR (OpenCV) в RGB
        image = cv2.cvtColor(image, cv2.COLOR_BGR2RGB)

        # Получаем класс автомобиля из названия файла
        vid_str = img_name.split('_')[0]
        label = self.id_to_idx[vid_str]

        # Применяем аугментации
        if self.transform:
            augmented = self.transform(image=image)
            image = augmented['image']

        return image, torch.tensor(label, dtype=torch.long)


# Пайплайн аугментаций для тренировочной выборки
train_transform = A.Compose([
    A.Resize(256, 256),  # Стандартное разрешение для ReID (иногда используют 256x128)
    A.HorizontalFlip(p=0.5),
    A.RandomBrightnessContrast(p=0.2),
    A.HueSaturationValue(p=0.2),
    A.CoarseDropout(num_holes_range=(1, 4), hole_height_range=(1, 32), hole_width_range=(1, 32), fill=0, p=0.5),  # Имитация перекрытий
    A.Normalize(mean=(0.485, 0.456, 0.406), std=(0.229, 0.224, 0.225)),  # Стандарт ImageNet
    ToTensorV2(),
])

# Пайплайн для валидации и теста (только ресайз и нормализация)
test_transform = A.Compose([
    A.Resize(256, 256),
    A.Normalize(mean=(0.485, 0.456, 0.406), std=(0.229, 0.224, 0.225)),
    ToTensorV2(),
])

if __name__ == "__main__":
    # Укажите путь к папке с кропами train выборки
    TRAIN_CROPS_DIR = os.path.join("../crops", "bounding_box_train")

    # Создаем датасет и даталоадер
    train_dataset = VehicleReIDDataset(image_dir=TRAIN_CROPS_DIR, transform=train_transform)
    train_loader = DataLoader(train_dataset, batch_size=32, shuffle=True, num_workers=4)

    print(f"Количество изображений в обучении: {len(train_dataset)}")
    print(f"Количество уникальных автомобилей (классов): {train_dataset.num_classes}")

    # Берем один батч для проверки
    images, labels = next(iter(train_loader))
    print(f"Размерность батча картинок: {images.shape}")  # Ожидаем [32, 3, 256, 256]
    print(f"Метки классов: {labels}")