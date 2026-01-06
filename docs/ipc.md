# IPC протоколы SparkOS

В SparkOS все взаимодействия между задачами происходят только через IPC сообщения ядра.
Ядро не интерпретирует `Kind` и полезную нагрузку, но система фиксирует общий контракт протоколов.

## Конверт сообщения

С точки зрения задач IPC выглядит так:

- `Kind` (`uint16`): тип сообщения (протокол).
- `Data[:Len]`: полезная нагрузка (байты), формат зависит от `Kind`.
- `Cap`: опциональный перенос capability (явный).

`From`/`To` выставляются ядром по endpoint’ам, переданным в `Context.Send*`.

## Request/Reply конвенция

Если клиент ожидает ответ:

- клиент создаёт свой reply endpoint и передаёт **reply capability** в поле `Cap` запроса;
- сервис отвечает в этот capability;
- для корреляции используется `requestID` (`u32`, little-endian) в payload (см. конкретные протоколы).

Пояснение:
- Если клиент создаёт отдельный reply endpoint на каждый запрос, `requestID` может быть `0`.
- Если клиент переиспользует один reply endpoint для многих запросов, `requestID` обязателен.

## Базовые Kind

Определены в `sparkos/proto`:

- `MsgLogLine`: однонаправленный лог.
- `MsgSleep`: запрос к time service (payload = `u32 requestID` + `u32 dt`, reply cap обязателен).
- `MsgWake`: ответ от time service (payload = `u32 requestID`).
- `MsgError`: универсальный ответ-ошибка.
- `MsgTermWrite`: best-effort VT100/ANSI bytes в терминал.
- `MsgTermClear`: очистка/сброс терминала.
- `MsgTermInput`: best-effort VT100/ANSI bytes от клавиатуры (для shell и подобных потребителей).

## Протокол: Logger

**MsgLogLine**

- Направление: client -> logger service (one-way).
- Payload: UTF-8 bytes (без завершающего `\n`).
- Ответ: отсутствует.
- Переполнение: best-effort, клиент может дропать при `SendErrQueueFull`.

## Протокол: Time

**MsgSleep**

- Направление: client -> time service.
- `Cap`: reply capability (право `RightSend`), куда time service отправит ответ.
- Payload (little-endian):
  - `u32 requestID`
  - `u32 dt` (ticks)
- Семантика:
  - `dt == 0`: немедленный ответ `MsgWake`.
  - если очередь ожиданий time service переполнена: ответ `MsgError` с `ErrOverflow`.

**MsgWake**

- Направление: time service -> reply endpoint.
- Payload: `u32 requestID` (little-endian).

## Протокол: Term

**MsgTermWrite**

- Направление: client -> term service (one-way).
- Payload: байты VT100/ANSI (поток), максимум `kernel.MaxMessageBytes` на сообщение.
- Переполнение: best-effort; клиент может дропать или ретраить (например, через `Context.BlockOnTick` или time-service sleep).

**MsgTermClear**

- Направление: client -> term service.
- Payload: пусто.

## Сервис: termkbd

`termkbd` — сервис, который владеет клавиатурой (`HAL.Input().Keyboard()`) и преобразует события в VT100/ANSI байты.

Текущая интеграция:
- `termkbd` отправляет результат как `MsgTermWrite` в term service.
- `termkbd` (альтернатива) отправляет результат как `MsgTermInput` в shell service.

## Универсальная ошибка (MsgError)

`MsgError` предназначен для request/reply протоколов.

Payload кодируется через `proto.ErrorPayload`:

- `u16 code` (`proto.ErrCode`)
- `u16 refKind` (`proto.Kind`) — `Kind` запроса, который не удалось обработать
- `detail[]byte` — необязательные байты, сервис-специфично (рекомендуется начинать с `u32 requestID`, если он был в запросе)

Рекомендуемое правило:
- если запрос невалиден/неавторизован/не может быть обработан, сервис отвечает `MsgError` в reply endpoint.

## Семантика переполнения (backpressure)

Каждый endpoint имеет фиксированную mailbox-очередь (сейчас `mailboxSlots = 8`).

- `Send`/`SendCap` **никогда не блокируют**.
- При переполнении очередь **не принимает** новое сообщение: `SendCapResult` возвращает `SendErrQueueFull`, сообщение не доставлено.
- `Recv` блокирует задачу только если очередь пуста; `TryRecv` никогда не блокирует.

Рекомендуемые политики на уровне протокола:
- Для best-effort потоков (лог/телеметрия) клиент может **дропать** сообщение при `SendErrQueueFull`.
- Для request/reply клиент должен **ретраить** (например, через time service sleep) или деградировать поведение.

## Ограничение размера

Payload должен помещаться в `kernel.MaxMessageBytes` (сейчас `128`).
Если payload больше лимита, ядро отвергает отправку: `SendErrPayloadTooLarge`.
