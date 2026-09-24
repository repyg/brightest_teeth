import os
import torch
import numpy as np
import albumentations as A
from albumentations.pytorch import ToTensorV2
from model import VehicleReIDModel


class VehicleFeatureExtractor:
    """
    Загружает веса один раз, принимает сырое изображение и BBox,
    возвращает вектор признаков.
    """
    def __init__(self, weights_path: str, num_classes: int = 1541, device: str = None):
        if device is None:
            self.device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
        else:
            self.device = torch.device(device)

        if not os.path.exists(weights_path):
            raise FileNotFoundError(f"Веса не найдены по пути: {weights_path}")

        # Инициализация модели
        self.model = VehicleReIDModel(num_classes=num_classes, model_name='resnet50', embedding_size=512)
        self.model.load_state_dict(torch.load(weights_path, map_location=self.device))
        self.model.to(self.device)
        self.model.eval()

        # пайплайн обработки
        self.transform = A.Compose([
            A.Resize(256, 256),
            A.Normalize(mean=(0.485, 0.456, 0.406), std=(0.229, 0.224, 0.225)),
            ToTensorV2(),
        ])

    @torch.no_grad()
    def extract_vector(self, image: np.ndarray, bbox: list) -> np.ndarray:
        """
        image: Изображение в формате numpy array (RGB)
        bbox: Координаты [x, y, w, h]
        """
        # вырезаем автомобиль
        x, y, w, h = [int(v) for v in bbox]
        y_max, x_max = image.shape[:2]
        x1, y1 = max(0, x), max(0, y)
        x2, y2 = min(x_max, x + w), min(y_max, y + h)

        crop = image[y1:y2, x1:x2]

        if crop.size == 0:
            # защита от битых координат: возвращаем нулевой вектор
            return np.zeros(512, dtype=np.float32)

        # применяем трансформации (ресайз + нормализация)
        tensor = self.transform(image=crop)['image']
        tensor = tensor.unsqueeze(0).to(self.device)  # Добавляем размерность батча [1, 3, 256, 256]

        # прогон через модель
        embedding = self.model(tensor)

        # возвращаем одномерный numpy-массив (вектор 512)
        return embedding.cpu().numpy()[0]