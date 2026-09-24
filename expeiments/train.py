import os
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import DataLoader
from tqdm import tqdm

# Импортируем классы из наших файлов
from expeiments.dataset import VehicleReIDDataset, train_transform
from core.model import VehicleReIDModel


def train():
    # Настройки
    NUM_EPOCHS = 10
    BATCH_SIZE = 32
    LEARNING_RATE = 3e-4

    # Абсолютный путь к кропам, которые мы нарезали ранее
    BASE_DIR = os.path.dirname(os.path.abspath(__file__))
    TRAIN_CROPS_DIR = os.path.join(BASE_DIR, "../crops", "bounding_box_train")

    # Устройство (видеокарта, если доступна)
    device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
    print(f"Используем устройство: {device}")

    # 1. Подготавливаем данные
    train_dataset = VehicleReIDDataset(image_dir=TRAIN_CROPS_DIR, transform=train_transform)
    train_loader = DataLoader(train_dataset, batch_size=BATCH_SIZE, shuffle=True, num_workers=4, pin_memory=True)
    num_classes = train_dataset.num_classes

    # 2. Инициализируем модель
    # Подключаем веса 'resnet50' и создаем классификатор ArcFace на наше количество машин
    model = VehicleReIDModel(num_classes=num_classes, model_name='resnet50', embedding_size=512)
    model = model.to(device)

    # 3. Функция потерь и Оптимизатор
    # Для ArcFace используется стандартная кросс-энтропия
    criterion = nn.CrossEntropyLoss()
    optimizer = optim.AdamW(model.parameters(), lr=LEARNING_RATE, weight_decay=1e-4)

    # Планировщик learning rate (немного снижаем шаг обучения в конце)
    scheduler = optim.lr_scheduler.CosineAnnealingLR(optimizer, T_max=NUM_EPOCHS)

    print(f"Начинаем обучение на {NUM_EPOCHS} эпох...")

    # 4. Цикл обучения
    for epoch in range(NUM_EPOCHS):
        model.train()
        running_loss = 0.0

        # tqdm для красивого прогресс-бара
        progress_bar = tqdm(train_loader, desc=f'Эпоха {epoch + 1}/{NUM_EPOCHS}')

        for images, labels in progress_bar:
            images = images.to(device)
            labels = labels.to(device)

            # Обнуляем градиенты
            optimizer.zero_grad()

            # Проход вперед
            outputs = model(images, labels)
            loss = criterion(outputs, labels)

            # Проход назад (обратное распространение)
            loss.backward()
            optimizer.step()

            running_loss += loss.item()
            # Обновляем текст в прогресс-баре
            progress_bar.set_postfix({'Loss': f'{loss.item():.4f}'})

        # Шаг планировщика
        scheduler.step()

        epoch_loss = running_loss / len(train_loader)
        print(f"Эпоха [{epoch + 1}/{NUM_EPOCHS}] завершена. Средний Loss: {epoch_loss:.4f}")

    # 5. Сохранение обученных весов
    os.makedirs('../core/weights', exist_ok=True)
    save_path = os.path.join('../core/weights', 'core/weights/reid_model_final.pth')
    torch.save(model.state_dict(), save_path)
    print(f"Обучение завершено! Веса сохранены в: {save_path}")


if __name__ == '__main__':
    train()