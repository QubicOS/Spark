# TEA — Tiny Embedded Audio

**Format Specification v1.0**

## Цель

Минимальный аудиоформат для MCU / TinyGo:

* детерминированный
* потоковый
* без malloc
* минимальный CPU
* предсказуемый тайминг

---

## Общие принципы

* **block-based**
* **fixed-point**
* **mono by default**
* декодирование без lookahead
* seek по блокам
* real-time safe

---

## Поддерживаемые кодеки (v1)

| ID   | Codec     |
| ---- | --------- |
| 0x01 | PCM16     |
| 0x02 | IMA-ADPCM |

(другие — только в v2)

---

## Файл: общая структура

```
[Header]
[Block 0]
[Block 1]
[Block 2]
...
```

---

## Header (фиксированный, 32 байта)

| Offset | Size | Type | Description             |
| ------ | ---- | ---- | ----------------------- |
| 0x00   | 4    | char | Magic = "TEA1"          |
| 0x04   | 2    | u16  | Sample rate (Hz)        |
| 0x06   | 1    | u8   | Channels (1 only in v1) |
| 0x07   | 1    | u8   | Codec ID                |
| 0x08   | 2    | u16  | Samples per block       |
| 0x0A   | 2    | u16  | Block size (bytes)      |
| 0x0C   | 4    | u32  | Total samples           |
| 0x10   | 2    | u16  | Flags                   |
| 0x12   | 14   | —    | Reserved (0)            |

---

## Flags

| Bit  | Meaning      |
| ---- | ------------ |
| 0    | Loop enabled |
| 1    | Has events   |
| 2–15 | Reserved     |

---

## Audio Block

### PCM16

```
[int16 sample 0]
[int16 sample 1]
...
```

### IMA-ADPCM

```
[int16 predictor]
[int8 step_index]
[adpcm nibbles...]
```

* каждый блок **самодостаточен**
* нет зависимости от предыдущего блока

---

## Event Stream (опционально)

Если `Flags.HasEvents = 1`:

```
[time:u16][type:u8][value:u8]
```

### Типы событий (v1)

| ID   | Event          |
| ---- | -------------- |
| 0x01 | Volume (0–255) |
| 0x02 | Loop start     |
| 0x03 | Loop end       |

---

## Потоковое декодирование

### Требования

* буфер ≤ 2 KB
* один блок за раз
* no allocation
* no GC in audio path

### Pipeline

```
FS → TEA Reader → Decoder → RingBuffer → DAC/PWM/I2S
```

---

## Реалтайм-гарантии

* декодирование блока < block_duration / 4
* блоки независимы → no jitter
* seek = block index

---

## Ограничения v1

* mono only
* no compression beyond ADPCM
* no filters
* no mixing

(всё это осознанно)

---

## Расширения (v2, не сейчас)

* stereo
* wavetable synth blocks
* MIDI-like events
* filters
* resampling

---

## Почему TEA хорош для Spark

* идеально ложится в **Audio Service**
* легко генерируется из Vector
* формат можно декодировать **и в Go, и в C**
* подходит для UI, игр, демо

---

## Коротко одной строкой

> **TEA — это ADPCM/PCM, сделанный правильно для MCU.**
