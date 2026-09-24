import math
import torch
import torch.nn as nn
import torch.nn.functional as F
import timm


class ArcFaceMarginProduct(nn.Module):
    """
    Слой ArcFace: увеличивает угловое расстояние между признаками разных классов.
    Заставляет сеть формировать математически уникальные эмбеддинги для каждого vehicle_id.
    """

    def __init__(self, in_features, out_features, s=30.0, m=0.30):
        super(ArcFaceMarginProduct, self).__init__()
        self.in_features = in_features
        self.out_features = out_features
        self.s = s
        self.m = m
        self.weight = nn.Parameter(torch.FloatTensor(out_features, in_features))
        nn.init.xavier_uniform_(self.weight)

        self.cos_m = math.cos(m)
        self.sin_m = math.sin(m)
        self.th = math.cos(math.pi - m)
        self.mm = math.sin(math.pi - m) * m

    def forward(self, inputs, labels):
        # косинусное сходство между признаками и весами классов
        cosine = F.linear(F.normalize(inputs), F.normalize(self.weight))
        sine = torch.sqrt(1.0 - torch.pow(cosine, 2)).clamp(0, 1)

        # маржа (угла m) к правильному классу
        phi = cosine * self.cos_m - sine * self.sin_m
        phi = torch.where(cosine > self.th, phi, cosine - self.mm)

        # one-hot векторы
        one_hot = torch.zeros(cosine.size(), device=inputs.device)
        one_hot.scatter_(1, labels.view(-1, 1).long(), 1)

        # применяем маржу только к истинному классу
        output = (one_hot * phi) + ((1.0 - one_hot) * cosine)
        output *= self.s
        return output


class VehicleReIDModel(nn.Module):
    def __init__(self, num_classes, model_name='resnet50', pretrained=True, embedding_size=512):
        super(VehicleReIDModel, self).__init__()
        # Backbone (вытягивает визуальные признаки)
        # num_classes=0 отрезает стандартную "голову" классификации ResNet
        self.backbone = timm.create_model(model_name, pretrained=pretrained, num_classes=0)
        in_features = self.backbone.num_features

        # Neck (проецирует признаки в плотный эмбеддинг фиксированной длины)
        self.neck = nn.Sequential(
            nn.Linear(in_features, embedding_size, bias=False),
            nn.BatchNorm1d(embedding_size),
        )

        # Head (ArcFace классификатор, нужен ТОЛЬКО для обучения)
        self.head = ArcFaceMarginProduct(embedding_size, num_classes, s=30.0, m=0.3)

    def forward(self, x, labels=None):
        features = self.backbone(x)
        embeddings = self.neck(features)

        # Режим инференса: если метки не переданы, возвращаем готовые векторы (для embeddings.npy)
        if labels is None:
            return F.normalize(embeddings, p=2, dim=1)

        # Режим обучения: возвращаем логиты для расчета CrossEntropyLoss
        logits = self.head(embeddings, labels)
        return logits


if __name__ == "__main__":
    model = VehicleReIDModel(num_classes=1541, model_name='resnet50', embedding_size=512)

    dummy_images = torch.randn(32, 3, 256, 256)
    dummy_labels = torch.randint(0, 1541, (32,))

    train_out = model(dummy_images, dummy_labels)
    print(f"Размерность выхода при обучении (логиты): {train_out.shape}")  # Ожидаем [32, 1541]

    infer_out = model(dummy_images)
    print(f"Размерность выхода при инференсе (эмбеддинги): {infer_out.shape}")  # Ожидаем [32, 512]