# Справочник API gashe

[English](REFERENCE.md) | Русский

Каждая экспортируемая функция, метод, тип и поле: что принимает и что возвращает. Когда какую
стратегию применять, описано в [README](README.ru.md#стратегии). Тот же текст есть в doc-комментариях,
поэтому `go doc github.com/eugeneliukindev/gashe.GetOrLoad` тоже его покажет.

- [Общие параметры](#общие-параметры)
- [Пакет gashe](#пакет-gashe)
  - [Создание кеша](#создание-кеша)
  - [Кеширование](#кеширование)
  - [Инвалидация](#инвалидация)
  - [Локи](#локи)
  - [Rate limit](#rate-limit)
  - [Circuit breaker](#circuit-breaker)
  - [Базовое API](#базовое-api)
  - [Кодеки](#кодеки)
  - [Бэкенды](#бэкенды)
  - [Ошибки](#ошибки)
- [Пакет gashe/backend/memory](#пакет-gashebackendmemory)
- [Пакет gashe/backend/nearcache](#пакет-gashebackendnearcache)
- [Пакет gashe/backend/redis](#пакет-gashebackendredis)
- [Пакет gashe/codec/msgpack](#пакет-gashecodecmsgpack)
- [Пакет gashe/backendtest](#пакет-gashebackendtest)

## Общие параметры

Эти параметры значат одно и то же в каждом вызове.

| Параметр | Тип | Значение |
|---|---|---|
| `ctx` | `context.Context` | Отменяет вызов и несет его дедлайн. |
| `c` | `*gashe.Cache` | Кеш, с которым идет работа. |
| `key` | `string` | Ключ записи, без `Policy.Prefix` (см. [Ключи](README.ru.md#ключи)). |
| `keys` | `[]string` или `...string` | Несколько ключей. |
| `ttl` | `time.Duration` | Сколько живет запись. В базовом API 0 значит "никогда не истекает", а отрицательное значение возвращает `ErrInvalidTTL`; локам и `Expire` нужно положительное. |
| `pattern` | `string` | Glob из Redis: `*`, `?`, `[abc]`, `[^a]`, `[a-z]`, `\` для экранирования. |
| `o` | `Policy[V]`, `BreakerOptions` | Опции вызова. Неверные возвращают `ErrInvalidOptions`. |
| `load` | функция | Ваш загрузчик: вызывается, когда кеш не может ответить. |

## Пакет gashe

`import "github.com/eugeneliukindev/gashe"`

### Создание кеша

#### New

```go
func New(b Backend, o Options) *Cache
```

Возвращает кеш поверх бэкенда `b`. Создайте один при старте и используйте везде; он безопасен для
одновременного использования. `b` он не закрывает. Паникует, если `b` равен nil или
`o.LockPollInterval` отрицательный.

#### Options

```go
type Options struct {
	Codec            Codec
	OnError          func(ctx context.Context, err error)
	OnResult         func(ctx context.Context, key string, r Result)
	LockPollInterval time.Duration
}
```

| Поле | Нулевое значение | Значение |
|---|---|---|
| `Codec` | `JSON{}` | Превращает значения в байты и обратно. |
| `OnError` | ошибки отбрасываются | Получает ошибки, которые кеш обошел: упавший бэкенд в `GetOrLoad`, `RateLimit`, фоновое обновление. |
| `OnResult` | не вызывается | Получает, как был отвечен каждый `GetOrLoad` и каждый ключ `GetOrLoadMany`, вместе с ключом. |
| `LockPollInterval` | 100 мс | Как часто `Lock` и `LockOnMiss` проверяют занятый лок. |

#### Cache.Shutdown

```go
func (c *Cache) Shutdown(ctx context.Context) error
```

Перестает запускать фоновые обновления и ждет уже запущенные. Возвращает `nil`, когда они
закончились, или `ctx.Err()`, если `ctx` завершился раньше. Кеш после этого продолжает работать.
Вызывайте перед закрытием бэкенда.

### Кеширование

#### GetOrLoad

```go
func GetOrLoad[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	load func(context.Context) (V, error),
) (V, error)
```

Возвращает сохраненный результат под `o.Prefix + key` или вызывает `load`, сохраняет его результат
и возвращает его.

- Принимает: `V` - тип результата; `o` говорит, как он кешируется; `load` возвращает настоящее
  значение.
- Возвращает: результат и `nil`; или ошибку `load` как есть (она не сохраняется, если на нее не
  ответил `StaleIfError` и ее не закешировал `Negative`); `o.Negative.Err`, пока закеширован
  отрицательный ответ; `ErrInvalidOptions` для неверной `o`.
- Если бэкенд упал, возвращает результат `load` без сохранения и передает ошибку в `OnError`.
- Одновременные вызовы с тем же ключом и типом в одном процессе делят одну загрузку.

#### GetOrLoadMany

```go
func GetOrLoadMany[V any](
	ctx context.Context,
	c *Cache,
	keys []string,
	o Policy[V],
	load func(ctx context.Context, missing []string) (map[string]V, error),
) (map[string]V, error)
```

`GetOrLoad` для многих ключей: одно чтение бэкенда, один вызов `load` с ключами, которых не было,
одна запись в бэкенд.

- Принимает: `keys` могут повторяться и идти в любом порядке; `load` получает каждый недостающий
  ключ один раз, по порядку, и возвращает найденные значения по ключам.
- Возвращает: map со всеми найденными или загруженными ключами; ключей, которые `load` не вернул,
  нет, и они не сохраняются. При ошибке `load` возвращает `nil` и эту ошибку.
- Поддерживает `TTL`, `TTLFunc`, `Jitter`, `Tags`, `CacheIf`, `Prefix`. `StaleWhileRevalidate`,
  `StaleIfError`, `LockOnMiss` и `Negative` возвращают `ErrInvalidOptions`.
- Читает и пишет те же записи, что и `GetOrLoad` с той же политикой. Одновременные вызовы не
  объединяются.

#### Reload

```go
func Reload[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	load func(context.Context) (V, error),
) (V, error)
```

Вызывает `load` и сохраняет его результат, что бы ни было сохранено, например после изменения
данных. Возвращает результат или ошибку `load`, которая не трогает сохраненный результат.

#### Policy

```go
type Policy[V any] struct {
	TTL                  time.Duration
	TTLFunc              func(V) time.Duration
	Jitter               float64
	StaleWhileRevalidate time.Duration
	StaleIfError         time.Duration
	StaleIf              func(error) bool
	LockOnMiss           time.Duration
	Negative             Negative
	Tags                 []string
	CacheIf              func(V) bool
	Prefix               string
}
```

Как кешируются результаты типа `V`. Задайте одну на каждый вид данных. Нулевое значение каждого
поля значит "выключено". Нужно задать хотя бы одно из `TTL`, `TTLFunc`, `StaleWhileRevalidate`,
`StaleIfError`. Длительности не могут быть отрицательными.

| Поле | Значение |
|---|---|
| `TTL` | Сколько результат свежий: отвечается без вызова загрузчика. |
| `TTLFunc` | Берет TTL из результата вместо `TTL`. Отрицательный TTL значит "не сохранять". |
| `Jitter` | Случайно разбрасывает TTL на эту долю, от 0 до меньше 1: `0.1` - это +-10%. |
| `StaleWhileRevalidate` | Сколько после TTL устаревший результат отвечается сразу, пока один вызов обновляет его в фоне. |
| `StaleIfError` | Сколько после TTL устаревший результат отвечает на ошибку загрузчика. |
| `StaleIf` | Какие ошибки загрузчика покрывает `StaleIfError`. nil значит любые. |
| `LockOnMiss` | При отсутствующем ключе грузит один процесс под локом, который держится не дольше этого; остальные ждут его результат. |
| `Negative` | Кеширует ошибки "не найдено", см. ниже. |
| `Tags` | Теги результата; `InvalidateTags` с любым из них делает его промахом. |
| `CacheIf` | Решает, сохранять ли результат. nil сохраняет каждый успешный результат. |
| `Prefix` | Ставится перед каждым ключом как есть, вместе с разделителем: `"user:"` хранит `"user:42"`. |

#### Negative

```go
type Negative struct {
	If  func(error) bool
	Err error
	TTL time.Duration
}
```

| Поле | Значение |
|---|---|
| `If` | Выбирает ошибки загрузчика для кеширования. nil выключает отрицательное кеширование. |
| `Err` | Ошибка, которую `GetOrLoad` возвращает, пока закеширован отрицательный ответ. Обязательна вместе с `If`. |
| `TTL` | Сколько хранится отрицательный ответ. Обязателен вместе с `If`. |

#### NotNil

```go
func NotNil[V any](v V) bool
```

Готовый `CacheIf`: false для nil-указателей, срезов, map, каналов, функций и интерфейсов, true для
всего остального.

#### Result

```go
type Result int

const (
	ResultHit                  // свежий сохраненный результат
	ResultMiss                 // загрузчик выполнился
	ResultStaleWhileRevalidate // устаревший результат, обновляется в фоне
	ResultStaleIfError         // устаревший результат вместо ошибки загрузчика
	ResultNegativeHit          // закешированный отрицательный ответ
)

func (r Result) String() string
```

То, что получает `Options.OnResult`. `String` возвращает `"hit"`, `"miss"`,
`"stale-while-revalidate"`, `"stale-if-error"` или `"negative-hit"`.

### Инвалидация

```go
func (c *Cache) InvalidateTags(ctx context.Context, tags ...string) error
func (c *Cache) Delete(ctx context.Context, key string) (bool, error)
func (c *Cache) DeleteMany(ctx context.Context, keys ...string) error
func (c *Cache) DeleteMatch(ctx context.Context, pattern string) error
```

| Метод | Принимает | Возвращает |
|---|---|---|
| `InvalidateTags` | теги для инвалидации | ошибку бэкенда. Каждый результат, сохраненный с одним из `tags`, становится промахом; одна запись на тег. |
| `Delete` | ключ как он хранится, с `Policy.Prefix` | был ли ключ. |
| `DeleteMany` | ключи как они хранятся | ошибку бэкенда. |
| `DeleteMatch` | шаблон | ошибку бэкенда. Обходит весь бэкенд. |

### Локи

```go
func (c *Cache) Lock(ctx context.Context, key string, ttl time.Duration) (*Lease, error)
func (c *Cache) TryLock(ctx context.Context, key string, ttl time.Duration) (*Lease, bool, error)
func (c *Cache) IsLocked(ctx context.Context, key string) (bool, error)
func (l *Lease) Extend(ctx context.Context, ttl time.Duration) error
func (l *Lease) Unlock(ctx context.Context) error
```

| Метод | Принимает | Возвращает |
|---|---|---|
| `Lock` | ключ лока; `ttl > 0`, после которого лок освобождается сам, если владелец умер | lease, когда лок взят; ждет, пока он занят, проверяя каждые `LockPollInterval`; `ctx.Err()`, если `ctx` завершился раньше. |
| `TryLock` | то же | lease и `true` или `nil, false`, если лок держит кто-то другой; не ждет. |
| `IsLocked` | ключ лока | держит ли его кто-нибудь. |
| `Lease.Extend` | новый `ttl > 0`, от текущего момента | `ErrLockLost`, если лок истек и его может держать кто-то другой. |
| `Lease.Unlock` | - | ошибку бэкенда. Освобождает лок, только если он все еще ваш, даже после отмены `ctx`. |

У `Lease` нет экспортируемых полей; его возвращают `Lock` и `TryLock`.

### Rate limit

```go
func (c *Cache) RateLimit(ctx context.Context, key string, limit Limit) error

type Limit interface{ /* SlidingWindow или FixedWindow */ }

type SlidingWindow struct {
	Calls int
	Per   time.Duration
}

func PerSecond(n int) SlidingWindow
func PerMinute(n int) SlidingWindow
func PerHour(n int) SlidingWindow

type FixedWindow struct {
	Calls  int
	Per    time.Duration
	BanFor time.Duration
}
```

`RateLimit` считает вызов для `key` и возвращает `ErrRateLimited`, когда ключ превысил `limit`,
иначе `nil`. Вызывайте его перед работой, которую он защищает. Если бэкенд упал, передает ошибку в
`OnError` и возвращает `nil`. Неверные лимиты возвращают `ErrInvalidOptions`.

| Тип, поле | Значение |
|---|---|
| `SlidingWindow.Calls`, `.Per` | Около `Calls` вызовов за любой отрезок `Per`, который кончается сейчас. Оба положительные. Отклоненные вызовы не считаются. |
| `PerSecond(n)`, `PerMinute(n)`, `PerHour(n)` | `SlidingWindow{Calls: n, Per: time.Second}` и так далее. |
| `FixedWindow.Calls`, `.Per` | Не больше `Calls` вызовов на отрезок `Per`, который начинается с первого вызова. Оба положительные. |
| `FixedWindow.BanFor` | Сколько ключ остается ограниченным после первого превышения. 0 значит `Per`. |

### Circuit breaker

```go
func (c *Cache) CircuitBreaker(key string, o BreakerOptions) (DoneFunc, error)

type DoneFunc func(err error)

type BreakerOptions struct {
	FailurePercent int
	MinCalls       int
	Window         time.Duration
	OpenFor        time.Duration
	HalfOpenCalls  int
	IsFailure      func(error) bool
}
```

`CircuitBreaker` проверяет breaker ключа `key` перед вызовом зависимости. Возвращает
`ErrCircuitOpen`, пока breaker открыт, или `DoneFunc`: вызовите ее один раз с ошибкой вашего вызова,
`nil` при успехе. `ctx` не нужен: breaker живет в памяти этого процесса. Для одного ключа используйте
одни и те же опции.

| Поле | Нулевое значение | Значение |
|---|---|---|
| `FailurePercent` | обязательно | Открывает breaker, когда эта доля вызовов в `Window` упала, от 1 до 99. |
| `MinCalls` | 1 | Сколько вызовов нужно в `Window`, прежде чем breaker может открыться. |
| `Window` | обязательно | Отрезок недавнего времени, за который считаются вызовы. |
| `OpenFor` | обязательно | Сколько открытый breaker отклоняет вызовы. |
| `HalfOpenCalls` | breaker просто закрывается после `OpenFor` | Сколько пробных вызовов пропускается после `OpenFor`: первый успех закрывает его, ошибка снова открывает. |
| `IsFailure` | любая ошибка | Какие ошибки считаются отказами. |

### Базовое API

Работает как map с TTL. Значения кодируются через `Options.Codec`; ключи используются как даны.

```go
func (c *Cache) Get(ctx context.Context, key string, dst any) error
func (c *Cache) Set(ctx context.Context, key string, v any, ttl time.Duration) error
func (c *Cache) SetIfAbsent(ctx context.Context, key string, v any, ttl time.Duration) (bool, error)
func (c *Cache) SetIfPresent(ctx context.Context, key string, v any, ttl time.Duration) (bool, error)
func (c *Cache) GetMany(ctx context.Context, keys ...string) ([]Value, error)
func (c *Cache) SetMany(ctx context.Context, values map[string]any, ttl time.Duration) error
func (c *Cache) Keys(ctx context.Context, pattern string) iter.Seq2[string, error]
func (c *Cache) All(ctx context.Context, pattern string) iter.Seq2[Value, error]
func (c *Cache) Exists(ctx context.Context, key string) (bool, error)
func (c *Cache) Incr(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error)
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error)
func (c *Cache) Ping(ctx context.Context) error
func (c *Cache) Backend() Backend
```

| Метод | Принимает | Возвращает |
|---|---|---|
| `Get` | `dst`, указатель, куда декодировать | `ErrNotFound` при промахе. |
| `Set` | значение `v` | ошибку. Заменяет значение и его TTL. |
| `SetIfAbsent` | значение `v` | сохранил ли: только если у ключа не было живой записи. |
| `SetIfPresent` | значение `v` | сохранил ли: только если у ключа была живая запись. |
| `GetMany` | ключи | по одному `Value` на ключ, по порядку; для отсутствующих ключей значения не найдены. |
| `SetMany` | значения по ключам | ошибку. Все получают один TTL. |
| `Keys` | шаблон | итератор по подходящим ключам; ошибка выдается один раз как error. На Redis может выдать ключ дважды. |
| `All` | шаблон | итератор по подходящим живым записям, читает пачками по 100. |
| `Exists` | ключ | есть ли у него живая запись. |
| `Incr` | `by`, может быть отрицательным; `ttl`, используется только при создании счетчика | новое значение. `ErrWrongType`, если значение не целое число. |
| `Expire` | `ttl > 0` | был ли ключ. |
| `TTL` | ключ | сколько осталось, `NoExpiry` для записи без TTL, `ErrNotFound` при промахе. |
| `Ping` | - | ошибку, если бэкенд недоступен. |
| `Backend` | - | бэкенд, для прямой работы с байтами. |

#### Value

```go
func (v Value) Key() string
func (v Value) Found() bool
func (v Value) Decode(dst any) error
```

Один результат `GetMany` или `All`. `Key` возвращает ключ, `Found` - была ли живая запись, `Decode`
декодирует ее в `dst` или возвращает `ErrNotFound`.

### Кодеки

```go
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

type JSON struct{}
type Gzip struct{ Inner Codec }
type Raw struct{ Inner Codec }
```

| Тип | Значение |
|---|---|
| `Codec` | Реализуйте его, чтобы хранить значения по-другому. |
| `JSON` | По умолчанию, `encoding/json`. |
| `Gzip` | Сжимает то, что кодирует `Inner`. nil `Inner` значит JSON. |
| `Raw` | Хранит `[]byte` и `string` как есть; остальные типы идут в `Inner`, nil значит JSON. |

Все три реализуют `Codec` через `Marshal` и `Unmarshal`.

### Бэкенды

```go
type Backend interface {
	Get(ctx context.Context, key string) ([]byte, error)
	GetMany(ctx context.Context, keys ...string) ([][]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	SetMany(ctx context.Context, values map[string][]byte, ttl time.Duration) error
	SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	SetIfPresent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	Delete(ctx context.Context, key string) (bool, error)
	DeleteMany(ctx context.Context, keys ...string) error
	DeleteMatch(ctx context.Context, pattern string) error
	Scan(ctx context.Context, pattern string) iter.Seq2[string, error]
	Exists(ctx context.Context, key string) (bool, error)
	Incr(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Unlock(ctx context.Context, key string, owner []byte) (bool, error)
	ExtendLock(ctx context.Context, key string, owner []byte, ttl time.Duration) (bool, error)
	Ping(ctx context.Context) error
	Close() error
}

const NoExpiry time.Duration = -1

func WithPrefix(b Backend, prefix string) Backend
```

`Backend` хранит байты; реализуйте его для своего хранилища и проверьте через `backendtest.Run`.
Методы совпадают с базовым API над байтами, плюс:

| Метод | Принимает | Возвращает |
|---|---|---|
| `GetMany` | ключи | значения в порядке ключей; для отсутствующего ключа - nil. |
| `Scan` | шаблон | итератор по подходящим ключам; ошибка выдается один раз, и обход останавливается. |
| `Unlock` | ключ лока, его владельца | удалил ли ключ: только если в нем `owner`. |
| `ExtendLock` | ключ лока, его владельца, `ttl > 0` | поставил ли TTL: только если в ключе `owner`. |
| `Close` | - | освобождает то, чем владеет бэкенд. |

`NoExpiry` - то, что `TTL` возвращает для записи без срока. `WithPrefix` возвращает бэкенд, который
хранит каждый ключ `b` под `prefix`, для нескольких сервисов в одном Redis; `Scan` выдает ключи без
него.

### Ошибки

Проверяйте через `errors.Is`.

| Ошибка | Когда возвращается |
|---|---|
| `ErrNotFound` | у ключа нет живой записи. |
| `ErrInvalidTTL` | TTL отрицательный или не положительный там, где должен быть. |
| `ErrWrongType` | `Incr` встретил значение, которое не целое число. |
| `ErrInvalidOptions` | неверные `Policy`, `BreakerOptions` или `Limit`. |
| `ErrLockLost` | `Lease.Extend` обнаружил, что лок истек. |
| `ErrRateLimited` | `RateLimit` отклонил вызов. |
| `ErrCircuitOpen` | `CircuitBreaker` отклонил вызов. |

## Пакет gashe/backend/memory

`import "github.com/eugeneliukindev/gashe/backend/memory"`

```go
func New(o Options) *Backend

type Options struct {
	MaxEntries    int
	MaxBytes      int64
	PurgeInterval time.Duration
}

var ErrTooLarge error

func (m *Backend) Len() int
func (m *Backend) Clear(context.Context) error
func (m *Backend) Close() error
```

`New` возвращает LRU в памяти процесса и запускает фоновую чистку; паникует при отрицательных
`MaxEntries` или `MaxBytes`. `Backend` реализует `gashe.Backend`.

| Поле | Нулевое значение | Значение |
|---|---|---|
| `MaxEntries` | 1000 | Максимум записей; вытесняется давно не использованная. |
| `MaxBytes` | без лимита | Максимум байт ключей и значений; вытесняются давно не использованные. |
| `PurgeInterval` | 1 с | Как часто чистятся истекшие записи. Отрицательное значение выключает чистку. |

`ErrTooLarge` возвращается для значения, которое одно не помещается в `MaxBytes`. `Len` возвращает
число записей, включая истекшие, которые еще не вычищены. `Clear` удаляет все, для тестов. `Close`
останавливает чистку и всегда возвращает `nil`.

## Пакет gashe/backend/nearcache

`import "github.com/eugeneliukindev/gashe/backend/nearcache"`

```go
func New(local, shared gashe.Backend, o Options) *Backend

type Options struct {
	LocalTTL time.Duration
}
```

`New` возвращает бэкенд, который читает через `local` (обычно `memory`) перед `shared` (обычно
Redis). Запись и удаление идут в `shared` и сбрасывают локальную копию. `LocalTTL` - сколько живет
локальная копия, 0 значит 5 секунд. `Backend` реализует `gashe.Backend`; его `Close` не закрывает
ни один из бэкендов.

## Пакет gashe/backend/redis

`import gasheredis "github.com/eugeneliukindev/gashe/backend/redis"`, отдельный модуль.

```go
func New(client goredis.UniversalClient) *Backend
```

Возвращает бэкенд поверх клиента go-redis v9: один узел, Sentinel или Cluster. Клиентом владеете вы
и закрываете его сами; `Backend.Close` ничего не делает. `Backend` реализует `gashe.Backend`. Ошибки
оборачиваются как `gashe/redis: <op>: <error>`; отсутствующий ключ становится `gashe.ErrNotFound`,
неверный тип - `gashe.ErrWrongType`.

## Пакет gashe/codec/msgpack

`import "github.com/eugeneliukindev/gashe/codec/msgpack"`, отдельный модуль.

```go
type Codec struct{}
```

`gashe.Codec` на основе MessagePack. Используйте `gashe.Options{Codec: msgpack.Codec{}}`. Ошибки
оборачиваются как `gashe/msgpack: marshal: <error>` и `gashe/msgpack: unmarshal: <error>`.

## Пакет gashe/backendtest

`import "github.com/eugeneliukindev/gashe/backendtest"`

```go
func Run(t *testing.T, newBackend func(t *testing.T) gashe.Backend)
```

Прогоняет набор контрактных тестов на вашем бэкенде. `newBackend` должен возвращать пустой бэкенд
для каждого подтеста.
