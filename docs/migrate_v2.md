# v1 → v2 database migration

## English

### Overview

Migration to the v2 schema is a manual operation performed by `awg-migrate v2`. The vpn-admin application never migrates a v1 database at startup.

The command creates a new v2 database and leaves the v1 source unchanged. If migration fails, the previous installation can continue using the original database.

Default paths:

```text
v1 source:      ~/.awg-admin
v2 destination: ~/.vpn-admin/db
```

### Safety guarantees

- The v1 source is opened read-only.
- Conversion uses a consistent BoltDB snapshot.
- The destination is written to a temporary file in the destination directory.
- The temporary file is atomically renamed only after the entire migration succeeds.
- A failed migration removes the temporary file and leaves the source and any existing destination untouched.
- An existing destination is never replaced without the explicit `-force` flag.
- Source and destination must be different files.

The database contains sensitive material, including SSH credentials, agent mTLS private keys and VPN peer keys. Database files are created with mode `0600`; protect them and any exported backups accordingly.

Agent certificates and private keys remain stored in BoltDB. `~/.vpn-admin/acme` is only the Let's Encrypt/autocert cache for the standalone web interface and is not part of database migration.

### Before migrating

1. Stop vpn-admin v1 before the operational cutover. Although the migrator reads a consistent snapshot, stopping v1 ensures that no subsequent changes are missing from the migrated copy.
2. Optionally create an additional backup using the v1 Backup function or:

   ```sh
   awg-migrate export -db ~/.awg-admin -out v1-backup.json
   ```

3. Make sure the destination filesystem has enough free space for a complete copy of the database.
4. If vpn-admin v2 is already running, stop it before replacing its database.

There is no dual-write between v1 and v2. Changes made in v1 after migration are not copied automatically; run the migration again if you need a newer snapshot.

### Run the migration

Build the migrator:

```sh
make migrate
```

Run it with the default paths:

```sh
awg-migrate v2
```

Or specify both files explicitly:

```sh
awg-migrate v2 -db ~/.awg-admin -out ~/.vpn-admin/db
```

Successful output has the form:

```text
migrated v1 /home/user/.awg-admin -> v2 /home/user/.vpn-admin/db; source was not modified
```

To replace an existing v2 destination atomically, add `-force`:

```sh
awg-migrate v2 -db ~/.awg-admin -out ~/.vpn-admin/db -force
```

Use `-force` only when replacing the destination is intentional. It does not modify the v1 source.

### What is converted

- Servers, including UUID, Location, SSH settings and agent mTLS material, are preserved.
- Users, peers, administrator credentials, agent sources and topology coordinates are preserved.
- Legacy interfaces become `awg/v1alpha1` resources with `Kind=Interface`; interface UUIDs are preserved.
- User peers become `Kind=Peer` resources with `interfaceRef` and `userRef`.
- Legacy tunnels and topology connections become `Link` records.
- New `resources`, `links`, `providers` and `meta` buckets are created.
- `meta.schemaVersion` is set to `2`.
- Private keys are not placed in resource `spec` documents.

### Start vpn-admin v2

Start vpn-admin normally after the destination has been created. If `~/.vpn-admin/db` exists, it takes precedence over the legacy `~/.awg-admin` path.

If no migrated database exists and the selected file still uses schema v1, vpn-admin v2 refuses to open it and asks for `awg-migrate v2`. It does not modify that file.

All migrated servers and their interfaces are visible in the topology, but every migrated server initially has the **Unmanaged** status. Migrated resources have `Pending` status and an unmanaged condition; previous v1 synchronization flags do not mark them as managed in v2.

### Complete agent migration

For each server:

1. Install or upgrade to the v2 agent through pack.
2. Install the AWG provider on that server through pack.
3. Let vpn-admin perform an agent-status check, or trigger it by refreshing the relevant view.

During this check, vpn-admin reads the agent manifest and submits the migrated resources through the AWG provider. Only after successful adoption does it clear the server's unmanaged flag. A subsequent successful agent health check reports the server as **OK** (green).

If the agent is unavailable, is still running v1, or does not expose the AWG provider, the server remains **Unmanaged**. Database migration itself does not connect to agents, run reconciliation or change running VPN services.

### Roll back to v1

The original v1 database is the rollback copy.

1. Stop vpn-admin v2.
2. Make sure no v2 process has the new database open.
3. Start vpn-admin v1 with the original `~/.awg-admin` file.

Do not point v1 at the migrated v2 database. Changes made only in v2 are not written back to the v1 file.

---

## Русский

### Общие сведения

Миграция на схему v2 выполняется вручную командой `awg-migrate v2`. Приложение vpn-admin никогда не мигрирует БД v1 автоматически при запуске.

Команда создаёт новую БД v2 и не изменяет исходную БД v1. Если миграция завершится ошибкой, прежняя установка сможет продолжить работу с исходным файлом.

Пути по умолчанию:

```text
Источник v1:    ~/.awg-admin
Назначение v2:  ~/.vpn-admin/db
```

### Гарантии безопасности

- Исходная БД v1 открывается только для чтения.
- Преобразование использует консистентный снимок BoltDB.
- Целевая БД записывается во временный файл в каталоге назначения.
- Временный файл атомарно переименовывается только после полного успеха миграции.
- При ошибке временный файл удаляется, а источник и существующая целевая БД не изменяются.
- Существующая целевая БД не перезаписывается без явно указанного флага `-force`.
- Источник и назначение должны быть разными файлами.

БД содержит секреты: SSH-учётные данные, приватные ключи mTLS агентов и ключи VPN-пиров. Файлы БД создаются с режимом `0600`; защищайте их и экспортированные резервные копии.

Сертификаты и приватные ключи агентов остаются в BoltDB. `~/.vpn-admin/acme` — только кэш Let’s Encrypt/autocert для HTTPS standalone-интерфейса; он не участвует в миграции БД.

### Перед миграцией

1. Остановите vpn-admin v1 перед переключением. Мигратор читает консистентный снимок, но остановка v1 гарантирует, что в перенесённую копию не попадут изменения, созданные уже после снимка.
2. Дополнительно можно создать резервную копию через Backup в v1 или командой:

   ```sh
   awg-migrate export -db ~/.awg-admin -out v1-backup.json
   ```

3. Убедитесь, что в целевой файловой системе достаточно места для полной копии БД.
4. Если vpn-admin v2 уже запущен, остановите его перед заменой его БД.

Dual-write между v1 и v2 отсутствует. Изменения, созданные в v1 после миграции, автоматически не переносятся; для получения более нового снимка повторите миграцию.

### Запуск миграции

Соберите утилиту:

```sh
make migrate
```

Запустите миграцию с путями по умолчанию:

```sh
awg-migrate v2
```

Или укажите оба файла явно:

```sh
awg-migrate v2 -db ~/.awg-admin -out ~/.vpn-admin/db
```

Пример успешного вывода:

```text
migrated v1 /home/user/.awg-admin -> v2 /home/user/.vpn-admin/db; source was not modified
```

Для атомарной замены существующей целевой БД v2 добавьте `-force`:

```sh
awg-migrate v2 -db ~/.awg-admin -out ~/.vpn-admin/db -force
```

Используйте `-force` только при необходимости заменить целевую БД. Этот флаг не изменяет источник v1.

### Что преобразуется

- Сохраняются серверы, включая UUID, Location, SSH-настройки и mTLS-материал агентов.
- Сохраняются пользователи, пиры, учётные данные администратора, источники агентов и координаты топологии.
- Прежние интерфейсы становятся ресурсами `awg/v1alpha1` с `Kind=Interface`; UUID интерфейсов сохраняются.
- Пиры пользователей становятся ресурсами `Kind=Peer` со ссылками `interfaceRef` и `userRef`.
- Прежние туннели и связи топологии становятся записями `Link`.
- Создаются buckets `resources`, `links`, `providers` и `meta`.
- В `meta.schemaVersion` записывается значение `2`.
- Приватные ключи не помещаются в документы `spec` ресурсов.

### Запуск vpn-admin v2

После создания целевой БД запустите vpn-admin обычным способом. Если файл `~/.vpn-admin/db` существует, он имеет приоритет над прежним путём `~/.awg-admin`.

Если мигрированной БД нет, а выбранный файл использует схему v1, vpn-admin v2 откажется открывать его и потребует выполнить `awg-migrate v2`. Сам файл при этом не изменяется.

Все перенесённые серверы и их интерфейсы отображаются в топологии, но каждый перенесённый сервер сначала имеет статус **Не управляется** (`Unmanaged`). Ресурсы получают статус `Pending` и условие unmanaged; прежние флаги синхронизации v1 не делают их управляемыми в v2.

### Завершение миграции агентов

Для каждого сервера:

1. Установите или обновите агента до v2 через pack.
2. Установите AWG-провайдер на этот сервер через pack.
3. Дождитесь проверки статуса агента vpn-admin или инициируйте её обновлением соответствующего представления.

Во время проверки vpn-admin читает manifest агента и передаёт перенесённые ресурсы AWG-провайдеру. Только после успешного принятия ресурсов признак unmanaged снимается. После следующей успешной проверки работоспособности сервер получает статус **OK** (зелёный).

Если агент недоступен, всё ещё работает на v1 или не предоставляет AWG-провайдер, сервер остаётся в статусе **Не управляется**. Сама миграция БД не подключается к агентам, не запускает reconcile и не изменяет работающие VPN-сервисы.

### Откат к v1

Исходная БД v1 является копией для отката.

1. Остановите vpn-admin v2.
2. Убедитесь, что ни один процесс v2 не держит новую БД открытой.
3. Запустите vpn-admin v1 с исходным файлом `~/.awg-admin`.

Не указывайте v1 мигрированную БД v2. Изменения, созданные только в v2, не записываются обратно в файл v1.
