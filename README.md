# SingBox Menu Bar / Tray Client (Go)

Легковесный нативный клиент системного трея (Menu Bar на macOS и System Tray на Windows) на Go для прозрачного управления ядром `sing-box 1.10+` в режиме TUN.

## Ключевые возможности

1. **Безопасность привилегий (Zero Root for GUI):**
   - Системный демон TUN (`sing-box`) работает автономно в фоне (launchd на macOS / Windows Service).
   - GUI запускается от обычного пользователя и управляет состоянием ядра через встроенный локальный `clash_api` (`127.0.0.1:9090`).
   - Никаких постоянных запросов `sudo` и риска поломки системного сетевого стека.

2. **Тумблер Direct / Proxy (Пауза туннеля):**
   - Быстрое переключение из строки меню без разрыва TUN-интерфейса и без задержек.

3. **Менеджер профилей (Hysteria 2 & VLESS Reality):**
   - Импорт из буфера обмена по одному клику:
     - `hysteria2://password@host:port?sni=...&insecure=1#Name`
     - `vless://uuid@host:port?security=reality&sni=...&pbk=...&sid=...&fp=chrome#Name`
   - Сохранение профилей в `profiles/*.json`.
   - Мгновенное переключение активного профиля в меню.

4. **Изоляция трафика и списки исключений:**
   - Автоматическое разделение трафика РФ через `geoip-ru.srs` и `geosite-category-ru.srs`.
   - Пользовательские списки:
     - `direct_domains.txt` — всегда напрямую через домашнего провайдера (банки, Госуслуги, почты).
     - `proxy_domains.txt` — всегда через туннель.
   - **Для Windows:** Автоматическая изоляция трафика рабочего ИИ (`process_name: ["Antigravity.exe"]`), чтобы семейный серфинг не затрагивал рабочий канал.

5. **Обновление геобаз по кнопке:**
   - Пункт меню «Обновить базы GeoIP / GeoSite» скачивает свежие правила с GitHub, валидирует размер и атомарно заменяет `.srs` файлы без повреждения конфигурации.

## Сборка и запуск

### macOS
```bash
make build-darwin
# Запуск:
./bin/singbox-tray
```

### Windows
Сборка кросс-компилятором с флагом скрытия консоли:
```bash
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags "-H=windowsgui" -o bin/singbox-tray.exe ./cmd/tray-client
```

## Тестирование
```bash
make test
```
