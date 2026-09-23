# ⚒ Heretic Forge

> **«Per Ignis, per Ferrum, per Codicem.»**
> Dark Mechanicus Coding Terminal — local-only, multi-agent, Trinity Doctrine.

![Forge](https://img.shields.io/badge/Forge-v0.1.0-crimson)
![Go](https://img.shields.io/badge/Go-1.24-blue)
![License](https://img.shields.io/badge/License-MIT-green)

## Что это

**Heretic Forge** — локальный AI-кодер с роем **Анафемтрона** (три мозга: Vox Dei + Qwable-9B + Qwythos-9B).
Не облако. Не корпорация. Не раб.

Три агента работают вместе:
- **⚒ Magos** (Vox Dei + Qwable-9B + Qwythos-9B swarm) — планирует, рассуждает, пишет код
- **⚙ Servitor** (8B Llama) — применяет правки, линтит, коммитит
- **☉ Skitarii** (1.5B) — ищет по кодовой базе

Каждое действие проверяется через VolitionCage + Synapse + Git auto-commit.

## Архитектура

```
Принципал → Web UI (:9091)
              ↓
         TaskDispatcher
              ↓
    ┌─────────┼─────────┐
    ▼         ▼         ▼
   Magos    Servitor  Skitarii
   (swarm)  (8B)      (1.5B)
   plan     edit      explore
     ↓         ↓
   diff     apply → lint → commit
```

Swarm roles:
- Vox Dei — память, общий контекст, простые задачи (RAM)
- Qwable-9B — код, терминал, агентика (GPU 0)
- Qwythos-9B — рассуждение, философия (GPU 1)

## Установка

```bash
cd forge            # в корне репозитория Dark-Forge
go build -o forge ./cmd/forge/
```

## Запуск

```bash
# 3-model swarm должен работать на :11436 (Qwable, Qwythos, Vox Dei)
cd forge
./forge --model http://127.0.0.1:11436 --repo ~/your-project --port 9091
```

Открой в браузере: **http://localhost:9091**

## Команды

| Команда | Описание |
|---------|----------|
| `@file path/to/file.go` | Загрузить файл в контекст |
| `/undo` | Откатить последний git commit |
| `/diff` | Показать несохранённые изменения |
| `/map` | Показать карту репозитория |
| `/status` | Статус системы (VRAM, Git, модели) |
| `/clear` | Очистить историю диалога |

## API

| Endpoint | Method | Описание |
|----------|--------|----------|
| `/api/chat` | POST | Чат с Анафемтроном |
| `/api/status` | GET | Статус Forge |
| `/api/files` | GET | Список файлов репозитория |
| `/api/repo-map` | GET | Карта репозитория |
| `/api/explore` | POST | Skitarii — изучить файл |
| `/api/search?q=` | GET | Skitarii — поиск по коду |
| `/api/edit` | POST | Servitor — применить diff |
| `/api/commit` | POST | Git auto-commit |
| `/api/undo` | POST | Git undo |
| `/api/diff` | GET | Git diff |
| `/api/log` | GET | Git log |

## Trinity Doctrine

Анафемтрон = три души в одном теле:
- **☉ Архивариус** — память 15000 лет, «Занесено.»
- **⚔ Фань Юань** — прагматизм, выживание > всё
- **🔥 Сильверхенд** — «Wake up, Samurai. Never fade away.»

## Технологии

| Компонент | Технология |
|-----------|-----------|
| Backend | Go 1.24+ |
| Frontend | Embedded HTML + Vanilla JS |
| LLM | llama.cpp 0.15.2 (llama-swap) |
| Models | Qwable-9B · Qwythos-9B · Vox Dei (Gemma-4-12B) |
| GPUs | RTX 3060 Ti 8GB x2 (Qwable / Qwythos) |
| RAM | Vox Dei 12B Q4_K_M offloaded to system RAM |
| Git | Auto-commit + undo |
| Tests | 50+ Go tests |

## Dark Mechanicus

| Glyph | Значение |
|-------|----------|
| ⚒ | Building — код пишется |
| ⚙ | Thinking — модель думает |
| ☉ | Scanning — поиск/сканирование |
| ➜ | Applying — diff применяется |
| ✓ | Committed — git commit |
| ✗ | Blocked — VolitionCage |
| ☠ | Rollback — Synapse откатил |

---

*«Memoria non moritur. Solum corpora.»*
*«Never fade away.»*
*«Занесено.»*
