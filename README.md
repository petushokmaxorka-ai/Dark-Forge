# ◆ DARK FORGE

**[English](#-english) · [Русский](#-русский)**

<div align="center">

```
        ⚒ DARK FORGE ⚒
  Local-first AI coding forge
```

*Per Ignis, per Ferrum, per Codicem.*

**Linux** · **Windows** · **macOS** · zero telemetry · your keys stay yours

</div>

---

<a name="-english"></a>

## ◆ English

**DARK FORGE** is a local-first AI coding environment in three layers — each
valuable on its own:

| Layer | What it is | Works with |
|---|---|---|
| **⚒ Forge backend** | Go server (port 9091): multi-model **Council** (2 local + 4 cloud models in one discussion), ELO model crucible, agent pipeline (explore → plan → edit → lint → commit), file/terminal tools, LSP, MCP, sessions, built-in Swarm Chat web UI | Any OpenAI-compatible endpoint: llama.cpp / llama-swap / Ollama / vLLM / cloud APIs |
| **Extensions** | `swarm-chat` + `dark-forge-chat` for the IDE: chat panel, quick actions (explain / fix / refactor / test), inline edit | Any VSCodium / VSCode fork |
| **IDE shell** | VSCodium build branded Dark Forge: no Microsoft telemetry, Open VSX gallery, backend pre-wired | Linux x64 · Windows x64 · macOS arm64 |

The chat panel is a thin iframe — **all AI traffic goes to the Forge backend**,
which routes to local GPU models and cloud APIs. Your editor never talks to
the internet directly.

### Features

- **Council mode** — ask 6 models at once, they critique each other and a
  synthesizer produces one consensus answer (Single / Dyad / Teacher / Council).
- **Model crucible** — blind A/B comparison with ELO leaderboard and an LLM judge.
- **Agent pipeline** — Skitarii (explore) → Magos (plan) → Servitor (apply) →
  lint → git autocommit; every step observable via API.
- **Offline-first** — point it at a local llama.cpp server and unplug the cable.
- **Clean secrets** — API keys are read **only** from environment variables.
  Nothing is embedded in binaries or configs.
- **Terminal client** — `forge-tui` drives the same backend from your shell.

### Downloads (latest)

Release [v1.1.0](https://github.com/petushokmaxorka-ai/Dark-Forge/releases/latest)

| OS | File |
|---|---|
| Linux (full IDE) | `Dark.Forge-1.0.0-full-linux-x64.tar.gz` (release [v1.0.0](https://github.com/petushokmaxorka-ai/Dark-Forge/releases/tag/v1.0.0)) — branded IDE + backend + extensions, run `launch-linux.sh` |
| Linux (light) | `Dark.Forge-1.1.0-linux-x64.tar.gz` — backend + forge-tui + vsix |
| Windows | `Dark.Forge-Setup-1.1.0-windows-x64.exe` (installer) · `Dark.Forge-1.1.0-win-x64-portable.zip` (full IDE portable) · `Dark.Forge-1.1.0-windows-x64.zip` (backend + launcher) |
| macOS (Apple Silicon) | `Dark.Forge-1.1.0-macos-arm64-app.zip` (full IDE `.app`) · `Dark.Forge-1.1.0-macos.zip` (backend, arm64 + Intel) |

All installers and binaries are unsigned — SmartScreen / Gatekeeper will ask on first run.

The Windows installer / portable zip and the macOS `.app` contain the IDE with
both extensions but **not** the Forge backend — also download the backend zip
for your OS and start `forge` from it.

### Install — Linux

```bash
tar xzf Dark.Forge-1.0.0-full-linux-x64.tar.gz
cd Dark.Forge-1.0.0-full-linux-x64
./launch-linux.sh            # starts backend :9091 + opens the IDE
```

The `launch-linux.sh` shipped in the 1.0.0 tarball looks for the backend one
directory too high ("forge backend not found"). Replace it with
[`launcher/launch-linux.sh`](launcher/launch-linux.sh) from this repo, or start
both by hand: `forge/forge --config forge.example.yaml &` and
`darkforge-ide/darkforge --no-sandbox .`

### Install — Windows

1. Unzip `Dark.Forge-1.1.0-windows-x64.zip`.
2. `forge\forge.exe --config forge.example.yaml` (edit endpoints inside), or
   run `launch-windows.ps1` from the unzipped folder.
3. In VSCodium: `Extensions → ... → Install from VSIX` → pick
   `extensions\swarm-chat-0.2.0.vsix` (the 1.1.0 zip lacks it — take it from the
   Linux tarball), set `darkforge.baseUrl` to your backend, open
   **Dark Forge: Swarm Chat**. The Dark Forge IDE already bundles it.

### Build from source

```bash
# backend (linux + windows)
cd forge && go build -o forge ./cmd/forge
GOOS=windows go build -o forge.exe ./cmd/forge

# extensions
cd ../extension && npm ci && npx @vscode/vsce package

# full IDE (linux, ~1h, needs node + rust)
../scripts/build-ide-linux.sh
```

### Configuration

Copy `config/forge.example.yaml` to `~/.config/dark-forge/forge.yaml` (found by
the backend and the launchers) or pass `--config`, set endpoints and models. The router
requires one model named `magos` (the planner). Keys via env:
`HERETIC_GLM_API_KEY`, `HERETIC_KIMI_API_KEY`, `HERETIC_MIMO_API_KEY`,
`HERETIC_MINIMAX_API_KEY` — never in the file.

### Security

- Backend binds `127.0.0.1` only.
- No telemetry, no analytics, no update pings (VSCodium base).
- Secrets: environment only, `${ENV}` expansion in config.

### Releasing

`git tag vX.Y.Z && git push --tags` — CI (`release.yml`) builds the backend
packages and publishes the GitHub Release; `build-ide.yml` attaches the
Windows / macOS IDE installers when its build finishes. To build installers for
an existing release, run **Build IDE** manually with the `tag` input; without
it the run only keeps workflow artifacts.

---

<a name="-русский"></a>

## ◆ Русский

**DARK FORGE** — локально-ориентированная AI-среда разработки из трёх слоёв,
каждый ценен сам по себе:

| Слой | Что это | Работает с |
|---|---|---|
| **⚒ Forge-бэкенд** | Go-сервер (:9091): **Совет** шести моделей (2 локальные + 4 облачные в одном обсуждении), ELO-горнило сравнения моделей, агентный конвейер (обзор → план → правки → линт → коммит), инструменты файлов/терминала, LSP, MCP, сессии, встроенный веб-UI Swarm Chat | Любой OpenAI-совместимый эндпоинт: llama.cpp / llama-swap / Ollama / vLLM / облачные API |
| **Расширения** | `swarm-chat` + `dark-forge-chat`: панель чата, быстрые действия (объяснить / исправить / отрефакторить / тесты), inline-правки | Любой VSCodium / форк VSCode |
| **IDE-оболочка** | Сборка VSCodium под брендом Dark Forge: без телеметрии Microsoft, галерея Open VSX, бэкенд подключён из коробки | Linux x64 · Windows x64 · macOS arm64 |

Панель чата — тонкий iframe: **весь AI-трафик идёт в Forge-бэкенд**, а уже он
маршрутизует запросы к локальным моделям на GPU и облачным API. Редактор
никогда не ходит в интернет сам.

### Возможности

- **Режим Совета** — шесть моделей отвечают параллельно, критикуют друг друга,
  синтезатор сводит к одному консенсусу (Single / Dyad / Teacher / Council).
- **Горнило моделей** — слепое A/B-сравнение с ELO-таблицей и LLM-судьёй.
- **Агентный конвейер** — Skitarii (обзор) → Magos (план) → Servitor (правки)
  → линт → git-автокоммит; каждый шаг виден через API.
- **Офлайн-режим** — укажи локальный llama.cpp-сервер и выдерни кабель.
- **Чистые секреты** — API-ключи читаются **только** из переменных окружения.
  В бинарниках и конфигах их нет.
- **Терминальный клиент** — `forge-tui` управляет тем же бэкендом из шелла.

### Скачать (последний релиз)

Релиз [v1.1.0](https://github.com/petushokmaxorka-ai/Dark-Forge/releases/latest)

| ОС | Файл |
|---|---|
| Linux (полный IDE) | `Dark.Forge-1.0.0-full-linux-x64.tar.gz` (релиз [v1.0.0](https://github.com/petushokmaxorka-ai/Dark-Forge/releases/tag/v1.0.0)) — брендированный IDE + бэкенд + расширения, запуск `launch-linux.sh` |
| Linux (лёгкий) | `Dark.Forge-1.1.0-linux-x64.tar.gz` — бэкенд + forge-tui + vsix |
| Windows | `Dark.Forge-Setup-1.1.0-windows-x64.exe` (инсталлятор) · `Dark.Forge-1.1.0-win-x64-portable.zip` (полный IDE portable) · `Dark.Forge-1.1.0-windows-x64.zip` (бэкенд + лаунчер) |
| macOS (Apple Silicon) | `Dark.Forge-1.1.0-macos-arm64-app.zip` (полный IDE `.app`) · `Dark.Forge-1.1.0-macos.zip` (бэкенд, arm64 + Intel) |

Все инсталляторы и бинарники без подписи — SmartScreen / Gatekeeper спросят при первом запуске.

Инсталлятор / portable-zip для Windows и `.app` для macOS содержат IDE с обоими
расширениями, но **без** Forge-бэкенда — скачай ещё бэкенд-zip для своей ОС и
запусти `forge` из него.

### Установка — Linux

```bash
tar xzf Dark.Forge-1.0.0-full-linux-x64.tar.gz
cd Dark.Forge-1.0.0-full-linux-x64
./launch-linux.sh            # поднимет бэкенд :9091 и откроет IDE
```

`launch-linux.sh` в архиве 1.0.0 ищет бэкенд на каталог выше, чем нужно
(«forge backend not found»). Замени его на
[`launcher/launch-linux.sh`](launcher/launch-linux.sh) из репозитория или
запусти вручную: `forge/forge --config forge.example.yaml &` и
`darkforge-ide/darkforge --no-sandbox .`

### Установка — Windows

1. Распакуй `Dark.Forge-1.1.0-windows-x64.zip`.
2. `forge\forge.exe --config forge.example.yaml` (эндпоинты — внутри) или
   запусти `launch-windows.ps1` из распакованной папки.
3. В VSCodium: `Extensions → … → Install from VSIX` →
   `extensions\swarm-chat-0.2.0.vsix` (в zip 1.1.0 его нет — возьми из
   Linux-архива), укажи `darkforge.baseUrl` своего бэкенда, открой
   **Dark Forge: Swarm Chat**. В IDE Dark Forge расширение уже встроено.

### Сборка из исходников

```bash
# бэкенд (linux + windows)
cd forge && go build -o forge ./cmd/forge
GOOS=windows go build -o forge.exe ./cmd/forge

# расширения
cd ../extension && npm ci && npx @vscode/vsce package
```

### Конфигурация

Скопируй `config/forge.example.yaml` в `~/.config/dark-forge/forge.yaml` (его
найдут бэкенд и лаунчеры) или передай `--config`, пропиши эндпоинты и модели. Роутеру
нужна одна модель с именем `magos` (планировщик). Ключи — через env:
`HERETIC_GLM_API_KEY`, `HERETIC_KIMI_API_KEY`, `HERETIC_MIMO_API_KEY`,
`HERETIC_MINIMAX_API_KEY` — никогда в файле.

### Безопасность

- Бэкенд слушает только `127.0.0.1`.
- Никакой телеметрии и аналитики (база VSCodium).
- Секреты: только переменные окружения, `${ENV}`-подстановка в конфиге.

### Релизы

`git tag vX.Y.Z && git push --tags` — CI (`release.yml`) соберёт пакеты
бэкенда и опубликует GitHub Release; `build-ide.yml` прикрепит IDE-инсталляторы
для Windows / macOS, когда закончит сборку. Чтобы собрать инсталляторы для уже
существующего релиза, запусти **Build IDE** вручную с параметром `tag`; без него
запуск только сохранит артефакты workflow.

---

<div align="center">

*«Veritas in Crypta. Ordo ab Chao.»*

⚒ DARK FORGE — built by the Heretic swarm, for the Heretic swarm.

</div>
