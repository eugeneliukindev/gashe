# gashe

[English](README.md) | Русский

Кеширование для Go-сервисов, которые работают в нескольких процессах: сохраняет результаты дорогих
вызовов, продолжает отвечать, когда зависимость тормозит или лежит, и защищает зависимости локами,
ограничением частоты (rate limit) и circuit breaker. Все это вокруг одного `*gashe.Cache`.

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/backend/redis # бэкенд Redis, зависит от go-redis v9
```

Нужен Go 1.23 или новее.

## Зачем

Кеш - это не только `Get` и `Set`. Рабочие сервисы натыкаются на одни и те же проблемы: холодный
ключ отправляет в базу сотню одинаковых запросов разом, популярный ключ истекает и каждый запрос
ждет перезагрузки, база падает и кешу нечем ответить, запросы несуществующих id каждый раз идут в
базу, тысячи ключей истекают в одну и ту же секунду. gashe решает каждую из них одной опцией или
одним методом и работает между процессами, которые делят один Redis.

## Возможности

- Типизированное кеширование: `gashe.GetOrLoad(ctx, c, key, opts, load)` возвращает ваш тип.
- Защита от cache stampede внутри процесса (автоматически) и между процессами (`LockOnMiss`).
- Stale-while-revalidate (`StaleWhileRevalidate`) и stale-if-error (`StaleIfError`) в одной записи.
- Отрицательное кеширование ответов "не найдено", разброс TTL против массового истечения.
- Пакетная загрузка списков (`GetOrLoadMany`): одно чтение кеша и один вызов загрузчика для всех
  недостающих ключей.
- Инвалидация по тегам за O(1) (`Tags`, `InvalidateTags`), без обхода бэкенда.
- Распределенные локи с продлением, скользящий и фиксированный лимиты, circuit breaker в памяти
  процесса.
- Бэкенды: LRU в памяти с TTL, Redis (один узел, Sentinel или Cluster) в отдельном модуле,
  двухуровневый near cache (память перед Redis), префиксы ключей для общего Redis.
- Сменный `Codec`: по умолчанию JSON, gzip для больших значений, сырые байты, MessagePack в
  отдельном модуле.
- Хуки для ошибок, которые кеш обошел (`OnError`), и для подсчета попаданий и промахов (`OnResult`).
- Если сломался сам кеш, ваш код продолжает работать.
- Без глобального состояния и без сторонних зависимостей в основном модуле; Go от 1.23 до самой
  новой версии.

## Содержание

- [Настройка](#настройка)
- [Бэкенды](#бэкенды)
- [Между процессами](#между-процессами)
- [Базовое API](#базовое-api)
- [Ключи](#ключи)
- [TTL](#ttl)
- [Что можно кешировать](#что-можно-кешировать)
- [Стратегии](#стратегии)
  - [Кеширование](#кеширование)
    - [Кеширование дорогих вызовов (cache-aside)](#кеширование-дорогих-вызовов-cache-aside)
    - [Всплески задержки при истечении (stale-while-revalidate)](#всплески-задержки-при-истечении-stale-while-revalidate)
    - [Отказ зависимости (stale-if-error)](#отказ-зависимости-stale-if-error)
    - [Cache penetration (отрицательное кеширование)](#cache-penetration-отрицательное-кеширование)
  - [Нагрузка на источник](#нагрузка-на-источник)
    - [Cache stampede внутри процесса](#cache-stampede-внутри-процесса)
    - [Cache stampede между процессами](#cache-stampede-между-процессами)
    - [N+1 запросов на списках (пакетная загрузка)](#n1-запросов-на-списках-пакетная-загрузка)
    - [Cache avalanche (разброс TTL)](#cache-avalanche-разброс-ttl)
    - [Горячие ключи и сетевые обращения (near cache)](#горячие-ключи-и-сетевые-обращения-near-cache)
  - [Хранение](#хранение)
    - [Пересечение ключей в общем Redis](#пересечение-ключей-в-общем-redis)
    - [Большие значения](#большие-значения)
  - [Защита зависимостей](#защита-зависимостей)
    - [Взаимное исключение между процессами (распределенный лок)](#взаимное-исключение-между-процессами-распределенный-лок)
    - [Перегрузка и злоупотребления (rate limiting)](#перегрузка-и-злоупотребления-rate-limiting)
    - [Каскадные отказы (circuit breaker)](#каскадные-отказы-circuit-breaker)
  - [Эксплуатация](#эксплуатация)
    - [Отказ кеша](#отказ-кеша)
    - [Доля попаданий и мониторинг](#доля-попаданий-и-мониторинг)
    - [Корректная остановка](#корректная-остановка)
  - [Известные ограничения](#известные-ограничения)
- [Инвалидация](#инвалидация)
  - [Инвалидация по тегам](#инвалидация-по-тегам)
- [Ошибки](#ошибки)
- [Как тестировать свой код](#как-тестировать-свой-код)
- [План](#план)
- [Разработка](#разработка)

## Пример

У библиотеки четыре части: **бэкенд**, где живут записи, **`gashe.Options`** для всего кеша,
**`gashe.Policy`** для каждого вида данных и сами вызовы. Пример ниже использует их все; `db`,
`payments` и `clientIP` - это ваш код.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct {
	ID   string
	Name string
}

var errNoUser = errors.New("user not found")

// Policy говорит, как кешируется один вид данных. Задайте ее один раз рядом с кодом загрузки.
var users = gashe.Policy[User]{
	Prefix:               "user:",          // хранится под "user:<id>"
	TTL:                  10 * time.Minute, // свежий 10 минут
	Jitter:               0.1,              // разброс истечения до 10%, чтобы пользователи не истекали разом
	StaleWhileRevalidate: time.Minute,      // минуту после: отвечать старым, обновлять в фоне
	StaleIfError:         time.Hour,        // база лежит: отвечать старым до часа
	LockOnMiss:           5 * time.Second,  // холодный ключ: грузит один процесс, остальные ждут
	Negative: gashe.Negative{ // "нет такого" тоже кешируется, и чужие id не доходят до базы
		If:  func(err error) bool { return errors.Is(err, errNoUser) },
		Err: errNoUser,
		TTL: time.Minute,
	},
	Tags: []string{"users"}, // c.InvalidateTags(ctx, "users") сбрасывает всех пользователей сразу
}

// GetOrLoadMany принимает только простые опции.
var userList = gashe.Policy[User]{Prefix: "user:", TTL: 10 * time.Minute, Tags: []string{"users"}}

func main() {
	ctx := context.Background()

	// Бэкенд - место, где живут записи: memory для одного процесса, Redis, чтобы делить их между
	// процессами: gasheredis.New(goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})).
	backend := memory.New(memory.Options{MaxBytes: 256 << 20})
	defer backend.Close()

	// Options говорят, как ведет себя весь Cache. Все поля необязательны.
	c := gashe.New(backend, gashe.Options{
		OnError:  func(ctx context.Context, err error) { slog.WarnContext(ctx, "cache", "error", err) },
		OnResult: func(ctx context.Context, key string, r gashe.Result) { /* считать попадания для метрик */ },
	})
	defer c.Shutdown(ctx) // дать фоновым обновлениям закончиться до закрытия бэкенда

	// Одно значение: из кеша или из загрузчика, и тогда оно сохраняется.
	u, err := gashe.GetOrLoad(ctx, c, "42", users, func(ctx context.Context) (User, error) {
		return db.User(ctx, "42")
	})
	fmt.Println(u, err)

	// Много значений: одно чтение кеша, потом один вызов загрузчика для недостающих.
	list, err := gashe.GetOrLoadMany(ctx, c, []string{"1", "2", "3"}, userList,
		func(ctx context.Context, missing []string) (map[string]User, error) {
			return db.Users(ctx, missing)
		})
	fmt.Println(list, err)

	// После записи: обновить одного пользователя или сбросить всех сразу.
	if _, err := gashe.Reload(ctx, c, "42", users, func(ctx context.Context) (User, error) {
		return db.User(ctx, "42")
	}); err != nil {
		fmt.Println(err)
	}
	if err := c.InvalidateTags(ctx, "users"); err != nil {
		fmt.Println(err)
	}

	// Защита зависимостей: rate limit, circuit breaker, лок.
	if err := c.RateLimit(ctx, "api:"+clientIP, gashe.PerMinute(100)); errors.Is(err, gashe.ErrRateLimited) {
		return // ответить 429
	}
	if done, err := c.CircuitBreaker("payments", gashe.BreakerOptions{
		FailurePercent: 50, MinCalls: 20, Window: time.Minute, OpenFor: 30 * time.Second,
	}); err == nil { // gashe.ErrCircuitOpen, пока платежный API падает
		done(payments.Charge(ctx))
	}
	if lease, ok, err := c.TryLock(ctx, "job:daily-report", 10*time.Minute); err == nil && ok {
		defer lease.Unlock(ctx)
		// ежедневный отчет делает только один процесс
	}
}
```

| Часть | Что это | Где задается |
|---|---|---|
| Бэкенд: `memory.New`, `gasheredis.New`, `nearcache.New` | где живут записи, а значит, какие процессы их делят | один раз при старте |
| `gashe.Options` | хуки ошибок и метрик, кодек, опрос локов | один раз, в `gashe.New` |
| `gashe.Policy[V]` | TTL и стратегии для одного вида данных | переменная пакета на каждый вид, передается в каждый вызов |
| `gashe.GetOrLoad`, `GetOrLoadMany`, `Reload` | кеширование | там, где вы загружаете данные |
| `c.InvalidateTags`, `c.Delete` | инвалидация | после записи |
| `c.Lock`, `c.TryLock`, `c.RateLimit`, `c.CircuitBreaker` | защита зависимостей | вокруг работы, которую они охраняют |
| `c.Get`, `c.Set`, ... | базовое API, как map с TTL | где угодно |
| `c.Shutdown` | ждет фоновые обновления | в конце `main` |

Каждая опция описана в [Настройке](#настройка) и [Стратегиях](#стратегии).

## Настройка

Все настраивается структурами опций. Поле, которое вы не указали, получает значение по умолчанию,
поэтому пустая структура всегда работает.

```go
mem := memory.New(memory.Options{
	MaxEntries:    10_000,      // хранить не больше стольких записей; 0 значит 1000
	MaxBytes:      256 << 20,   // и не больше стольких байт ключей и значений; 0 значит без лимита
	PurgeInterval: time.Minute, // как часто выбрасывать истекшие записи; 0 значит раз в секунду
})
defer mem.Close()

c := gashe.New(mem, gashe.Options{
	Codec:            gashe.JSON{},          // как значения превращаются в байты; не указывайте для JSON
	OnError:          onError,               // ошибки, которые кеш обошел
	OnResult:         onResult,              // как был отвечен каждый вызов GetOrLoad
	LockPollInterval: 20 * time.Millisecond, // как часто ждущий Lock проверяет лок; 0 значит 100 мс
})
```

| Поле `gashe.Options` | Для чего |
|---|---|
| `Codec` | Превращает ваши значения в байты для бэкенда и обратно. JSON, если не задан другой. |
| `OnError` | `GetOrLoad` и `RateLimit` никогда не падают только из-за того, что сломался кеш: они продолжают работать без него. Ошибка кеша не теряется, она приходит в эту функцию. Без нее такие ошибки отбрасываются. |
| `OnResult` | Получает `gashe.ResultHit`, `ResultMiss`, `ResultStaleWhileRevalidate`, `ResultStaleIfError` или `ResultNegativeHit` для каждого вызова `GetOrLoad` вместе с ключом. Нужен для метрик доли попаданий. |
| `LockPollInterval` | Как часто `Lock` и `LockOnMiss` проверяют, освободился ли занятый лок. Уменьшайте для локов, которые держат миллисекунды, увеличивайте для долгих задач, чтобы меньше нагружать Redis. 0 значит 100 мс. |

Создайте один `Cache` при старте и используйте его везде; он безопасен для одновременного
использования. Бэкенд он не закрывает: закрывайте бэкенд там, где создали, после `c.Shutdown`
(см. [Корректная остановка](#корректная-остановка)).

## Бэкенды

### В памяти

`github.com/eugeneliukindev/gashe/backend/memory` хранит записи в памяти процесса: LRU фиксированного
размера, где у каждой записи может быть свой TTL. Истекшие записи никогда не возвращаются; они
удаляются при чтении и фоновой чисткой каждые `PurgeInterval`. Байты копируются при записи и при
чтении, поэтому изменение значения после `Set` или `Get` никогда не меняет кеш. У каждого процесса
свой кеш в памяти; чтобы делить данные между процессами, используйте Redis.

Шаблоны в `Keys`, `All` и `DeleteMatch` используют синтаксис glob из Redis: `*`, `?`, `[abc]`,
`[^a]`, `[a-z]` и `\` для экранирования.

> [!WARNING]
> `MaxEntries` сам по себе считает записи, а не байты: 10 000 записей по 1 МБ - это 10 ГБ памяти.
> Задайте и `MaxBytes`. Он считает только байты ключей и значений, без накладных расходов Go, так что
> оставьте запас. Значение больше `MaxBytes` не сохраняется: `Set` возвращает `memory.ErrTooLarge`,
> а `GetOrLoad` передает ее в `OnError`. У каждого процесса своя копия, и при перезапуске все
> теряется.

### Redis

`github.com/eugeneliukindev/gashe/backend/redis` - отдельный модуль поверх
[go-redis v9](https://github.com/redis/go-redis), поэтому основной модуль остается без зависимостей.
Клиент go-redis вы создаете и закрываете сами: бэкенд им не владеет. Таймауты, размер пула, TLS,
Sentinel и Cluster - это опции go-redis.

```go
import (
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

client := goredis.NewUniversalClient(&goredis.UniversalOptions{
	Addrs:        []string{"localhost:6379"},
	ReadTimeout:  100 * time.Millisecond, // медленный кеш не должен замедлять сервис
	WriteTimeout: 100 * time.Millisecond,
})
defer client.Close()
c := gashe.New(gasheredis.New(client), gashe.Options{})
```

Заметки:

- Redis-совместимые серверы работают через тот же модуль: интеграционные тесты проходят на Redis 7.0,
  Valkey 8, Dragonfly и KeyDB.
- В Cluster `Keys`, `All` и `DeleteMatch` обходят все master-узлы, а операции с несколькими ключами
  отправляются по одной команде на ключ, поэтому ключи могут лежать в разных слотах.
- `Keys` и `All` могут выдать ключ больше одного раза, как это делает Redis `SCAN`.
- Ошибки Redis оборачиваются один раз, в виде `gashe/redis: <op>: <error>`. Отсутствующий ключ
  становится `gashe.ErrNotFound`; ответ `WRONGTYPE` или нецелое значение в `Incr` -
  `gashe.ErrWrongType`.

> [!CAUTION]
> `Keys`, `All` и `DeleteMatch` обходят всю базу через `SCAN`, в Cluster - каждый master. На
> большом Redis это долго и нагружает его; для инвалидации лучше `InvalidateTags`.

### Двухуровневый (near cache)

`github.com/eugeneliukindev/gashe/backend/nearcache` ставит небольшой кеш в памяти перед общим бэкендом.
См. [Горячие ключи и сетевые обращения](#горячие-ключи-и-сетевые-обращения-near-cache).

### Свой бэкенд

Реализуйте `gashe.Backend`. Он работает только с байтами: `Cache` кодирует значения до того, как
они попадут в бэкенд. Точный контракт описан в комментариях к методам в `backend.go`: `ttl`, равный
0, означает "без срока", промах - это `gashe.ErrNotFound`, а каждый метод должен быть безопасен для
одновременного вызова. Чтобы изменить поведение бэкенда, оберните его: встройте `gashe.Backend` в
структуру и переопределите нужные методы.

Проверьте свой бэкенд тем же набором тестов контракта, который проходят встроенные бэкенды:

```go
func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // новый пустой бэкенд для каждого подтеста
	})
}
```

## Между процессами

Что общее, решает бэкенд, а не метод. Ниже **все процессы** - это все процессы, которые используют
тот же Redis (или другой общий бэкенд), а **этот процесс** - только процесс, где выполняется вызов.
У стратегий в [Стратегиях](#стратегии) есть такая же строка **Область**.

| Возможность | С Redis | С `memory` |
|---|---|---|
| Результаты `GetOrLoad`, `GetOrLoadMany`, `Get`, `Set` | все процессы | этот процесс |
| Отрицательные ответы (`Negative`) | все процессы | этот процесс |
| Единственное фоновое обновление `StaleWhileRevalidate` | одно на все процессы | одно на процесс |
| `LockOnMiss`: один загрузчик для холодного ключа | один на все процессы | один на процесс |
| `Lock`, `TryLock`, `Lease.Extend` | все процессы | этот процесс |
| `RateLimit` | один лимит на все процессы | свой лимит в каждом процессе |
| `Tags`, `InvalidateTags` | все процессы; через `nearcache` - в течение `LocalTTL` | этот процесс |
| Схлопывание одновременных вызовов `GetOrLoad` | этот процесс | этот процесс |
| `CircuitBreaker` | этот процесс | этот процесс |
| `OnResult`, `OnError`, `Shutdown` | этот процесс | этот процесс |
| Локальный уровень `nearcache` | этот процесс | - |

## Базовое API

```go
c.Set(ctx, "key", v, time.Hour)             // error; ttl 0 = без срока, старый TTL заменяется
c.SetIfAbsent(ctx, "key", v, time.Hour)     // (bool, error); только если ключа нет
c.SetIfPresent(ctx, "key", v, time.Hour)    // (bool, error); только если ключ есть
c.SetMany(ctx, map[string]any{"a": 1}, ttl) // error

c.Get(ctx, "key", &dst)                     // error; gashe.ErrNotFound при промахе
values, err := c.GetMany(ctx, "a", "b")     // []gashe.Value, в порядке ключей
values[0].Found()                           // bool
values[0].Decode(&dst)                      // error; gashe.ErrNotFound, если не найдено

for key, err := range c.Keys(ctx, "user:*") {} // все подходящие ключи
for v, err := range c.All(ctx, "user:*") {}    // все подходящие записи: v.Key(), v.Decode(&dst)

c.Incr(ctx, "counter", 1, time.Minute)      // (int64, error); ttl только при создании ключа
c.Exists(ctx, "key")                        // (bool, error)
c.Delete(ctx, "key")                        // (bool, error)
c.DeleteMany(ctx, "a", "b")                 // error
c.DeleteMatch(ctx, "user:*")                // error
c.Expire(ctx, "key", time.Minute)           // (bool, error); ttl должен быть положительным
c.TTL(ctx, "key")                           // (time.Duration, error); gashe.NoExpiry, если без срока
c.Ping(ctx)                                 // error

c.Backend()                                 // gashe.Backend, для работы с сырыми байтами
```

Целое, записанное через `Incr`, читается через `Get` в `int`, а JSON-число, записанное через `Set`,
можно увеличивать через `Incr`.

## Ключи

Ключи - это строки, которые вы собираете в коде из аргументов вызова:

```go
gashe.GetOrLoad(ctx, c, fmt.Sprintf("user:%d:lang:%s", id, lang), users, load)
gashe.GetOrLoad(ctx, c, fmt.Sprintf("token:%x", sha256.Sum256([]byte(token))), tokens, load)
```

Ключ, который доходит до Redis, складывается из трех частей, снаружи внутрь:

| Часть | Кто задает | Для чего | Пример |
|---|---|---|---|
| префикс бэкенда | `gashe.WithPrefix(backend, "orders:prod:")` | несколько сервисов или окружений в одном Redis | `orders:prod:` |
| префикс политики | `Policy.Prefix` | один вид данных и его версия | `users:v2:` |
| ключ | аргумент `key` | один элемент | `42` |

```go
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), "orders:prod:"), gashe.Options{})
users := gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"}
gashe.GetOrLoad(ctx, c, "42", users, load) // сохраняется как "orders:prod:users:v2:42"
```

Оба префикса используются как есть, разделитель не добавляется. Лимиты хранятся под `srl:` и
`rate_limit:`, поколения тегов - под `gashe:tag:`, поэтому они никогда не пересекаются с вашими
записями. Для разных типов значений нужны разные ключи: чтение ключа как другого типа - ошибка
декодирования, которую `GetOrLoad` считает промахом и загружает заново.

Служебные ключи рядом с записью: `<key>:refresh` (лок фоновой перезагрузки, максимум минута) и
`<key>:load` (`LockOnMiss`).

> [!WARNING]
> Два места в коде, которые используют один ключ с разными типами, заставляют друг друга
> промахиваться на каждом вызове: каждая перезагрузка затирает другой тип. Дайте каждому типу свой
> ключ или `Prefix`.

## TTL

TTL - обычные значения `time.Duration`.

- В `GetOrLoad` `TTL` - это сколько результат свежий, а запись хранится `TTL` плюс более длинное из
  `StaleWhileRevalidate` и `StaleIfError`. `TTLFunc` выбирает TTL для каждого результата, а `Jitter`
  его разбрасывает.
- В базовом API (`Set`, `SetIfAbsent`, `Incr`, ...) TTL, равный 0, означает "никогда не истекает",
  а отрицательный возвращает `gashe.ErrInvalidTTL`. `Set` всегда заменяет старый TTL; `Incr`
  использует свой TTL только при создании счетчика.

## Что можно кешировать

Все, что умеет кодировать кодек. С кодеком по умолчанию `gashe.JSON` это значения, которые
`encoding/json` может закодировать и раскодировать обратно в тот же тип: экспортируемые поля
структур, map со строковыми ключами, слайсы, числа, строки и bool. Неэкспортируемые поля не
сохраняются, а `GetOrLoad` возвращает значение таким, каким оно читается из кеша, поэтому вы
увидите это сразу, а не только при попадании в кеш.

Чтобы хранить другие типы, реализуйте `gashe.Codec` или добавьте своему типу `MarshalJSON` и
`UnmarshalJSON`.

## Стратегии

Каждый раздел называет проблему, на которую натыкаются кеши, говорит, как gashe ее решает, и
показывает настройку. Большинство решений - опции одного `gashe.Policy[V]`, и их можно
сочетать.

### Кеширование

#### Кеширование дорогих вызовов (cache-aside)

**Проблема.** Один и тот же дорогой вызов (запрос в базу, запрос к API) делается снова и снова с
одними и теми же аргументами.

**Решение.** `gashe.GetOrLoad` возвращает сохраненный результат, пока он свежий; иначе вызывает
вашу функцию (**загрузчик**), сохраняет результат на `TTL` и возвращает его. Ошибка загрузчика
возвращается как есть и никогда не сохраняется.

```go
var users = gashe.Policy[User]{TTL: time.Hour} // правила кеширования пользователей; задаются один раз

u, err := gashe.GetOrLoad(ctx, c, "user:42", users, func(ctx context.Context) (User, error) {
	return db.LoadUser(ctx, 42) // выполняется, только когда кеш не может ответить
})
```

`GetOrLoad` - функция, а не метод `c`, потому что до Go 1.27 у методов не может быть параметров
типа. Опции, которые не могут работать, возвращают `gashe.ErrInvalidOptions`.

Трех необязательных полей `Policy` хватает для большинства случаев; остальные добавляют следующие
разделы.

- **`CacheIf`** - сохранять результат, только если он того стоит. Без него сохраняется каждый
  успешный результат.

  ```go
  var search = gashe.Policy[[]Item]{
  	TTL:     time.Minute,
  	CacheIf: func(items []Item) bool { return len(items) > 0 }, // не кешировать пустые результаты
  }
  ```

  `gashe.NotNil` - готовое правило для указателей, слайсов и map:
  `gashe.Policy[*User]{TTL: time.Hour, CacheIf: gashe.NotNil[*User]}`.

- **`TTLFunc`** - брать TTL из самого результата, когда результат знает, сколько он действителен.
  Отрицательный TTL значит "не сохранять".

  ```go
  // Токен доступа кешируется до минуты до своего истечения.
  var tokens = gashe.Policy[Token]{
  	TTLFunc: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute },
  }
  ```

- **`Prefix`** - строка по вашему выбору, которая ставится перед каждым ключом этой политики как
  есть (разделитель пишите сами). Нужна, чтобы версионировать закешированные данные: когда у `User`
  появляются новые поля, поменяйте `"users:v1:"` на `"users:v2:"`, и старые записи больше не
  читаются в новый тип. Чтобы развести несколько сервисов в одном Redis, используйте
  `gashe.WithPrefix`, см. [Ключи](#ключи).

  ```go
  var users = gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"} // сохраняет "users:v2:42"
  u, err := gashe.GetOrLoad(ctx, c, "42", users, loadUser)
  ```

**Область.** Сохраненные результаты видят все процессы, которые используют тот же Redis; с `memory`
- только этот процесс.

**Пример.** Пользователь из PostgreSQL, кеш на час.

```go
var users = gashe.Policy[User]{TTL: time.Hour}

func (s *Service) User(ctx context.Context, id int64) (User, error) {
	return gashe.GetOrLoad(ctx, s.cache, fmt.Sprint("user:", id), users, func(ctx context.Context) (User, error) {
		var u User
		err := s.db.QueryRowContext(ctx, "SELECT id, name, email FROM users WHERE id = $1", id).
			Scan(&u.ID, &u.Name, &u.Email)
		return u, err
	})
}
```

#### Всплески задержки при истечении (stale-while-revalidate)

**Проблема.** Популярный ключ истекает; пока медленный загрузчик не закончит, ждет каждый запрос.

**Решение.** `StaleWhileRevalidate`, названный по директиве `Cache-Control` из RFC 5861: столько
времени после `TTL` устаревший результат отдается сразу, а ровно
один вызывающий во всех процессах перезагружает его в фоне. Никто не ждет загрузчика. Неудачная
перезагрузка оставляет устаревший результат и уходит в `OnError`; зависшая перезагрузка
останавливается через минуту.

```go
// Свежий 10 минут. Следующие 2 минуты вызывающие сразу получают устаревший результат, а один из них
// перезагружает его в фоне.
var feed = gashe.Policy[Feed]{TTL: 10 * time.Minute, StaleWhileRevalidate: 2 * time.Minute}
```

С `TTL: 0` и только `StaleWhileRevalidate` каждый вызов отвечает сразу и запускает перезагрузку в фоне;
ключ, который никто не прочитал за окно, истекает.

**Область.** Устаревший ответ берется из сохраненной записи, поэтому следует за бэкендом. Фоновое
обновление идет одно на все процессы с Redis и одно на процесс с `memory`.

**Пример.** Персональная лента из медленного сервиса рекомендаций: после прогрева его больше никто не ждет.

```go
var feeds = gashe.Policy[[]Item]{TTL: 5 * time.Minute, StaleWhileRevalidate: 30 * time.Minute}

func (s *Service) Feed(ctx context.Context, userID string) ([]Item, error) {
	return gashe.GetOrLoad(ctx, s.cache, "feed:"+userID, feeds, func(ctx context.Context) ([]Item, error) {
		return s.recommender.Feed(ctx, userID) // вызов на 300 мс, теперь он идет в фоне
	})
}
```

> [!CAUTION]
> Фоновая перезагрузка выполняется, когда запрос, который ее запустил, уже закончился. Ее `ctx`
> сохраняет значения запроса, но не его отмену, и получает дедлайн в одну минуту. Не используйте в
> загрузчике ресурсы, которые живут только пока идет запрос, например открытую транзакцию базы.

#### Отказ зависимости (stale-if-error)

**Проблема.** База или API лежит, и кешу нечем ответить, когда записи истекут.

**Решение.** `StaleIfError`, вторая директива из RFC 5861: столько времени после `TTL` устаревший
результат отвечает вместо ошибки загрузчика. Сначала все равно выполняется загрузчик; только если он
упал, вы получаете устаревший
результат и nil вместо ошибки, а ошибка загрузчика уходит в `OnError`. `StaleIf` ограничивает,
какие ошибки покрываются.

```go
var rates = gashe.Policy[Rates]{
	TTL:            5 * time.Minute,
	StaleIfError: time.Hour,
	StaleIf:     func(err error) bool { return errors.Is(err, ErrAPIDown) }, // nil = любая ошибка
}
```

С `TTL: 0` и только `StaleIfError` загрузчик выполняется на каждом вызове, а последний удачный
результат отвечает на его ошибки (кеш на случай отказа).

**Область.** Устаревший ответ берется из сохраненной записи: с Redis им может ответить любой
процесс, с `memory` - только этот.

**Пример.** Курсы валют из внешнего HTTP API: если провайдер лежит, последние курсы отдаются до шести часов.

```go
var rates = gashe.Policy[map[string]float64]{TTL: time.Minute, StaleIfError: 6 * time.Hour}

func (s *Service) Rates(ctx context.Context) (map[string]float64, error) {
	return gashe.GetOrLoad(ctx, s.cache, "rates:usd", rates, func(ctx context.Context) (map[string]float64, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://rates.example.com/usd", nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, err // провайдер лежит: вместо ошибки отдаются последние курсы
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("rates: %s", resp.Status)
		}
		var out map[string]float64
		return out, json.NewDecoder(resp.Body).Decode(&out)
	})
}
```

> [!WARNING]
> Ответ может быть возрастом до `TTL + StaleIfError`. Не используйте это для данных, где устаревший
> ответ не опоздавший, а неверный: права доступа, балансы, цены при оплате.

#### Cache penetration (отрицательное кеширование)

**Проблема.** Запросы данных, которых нет (удаленный пользователь, неверный id, парсер, который
перебирает id подряд), никогда не кешируются, и каждый доходит до базы.

**Решение.** `Negative` ненадолго кеширует "не найдено". Пока ответ в кеше, `GetOrLoad` возвращает
выбранную вами ошибку, так что ваш код продолжает проверять тот же sentinel.

```go
var users = gashe.Policy[User]{
	TTL: time.Hour,
	Negative: gashe.Negative{
		If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) }, // какие ошибки значат "данных нет"
		Err: sql.ErrNoRows, // что возвращает GetOrLoad, пока ответ в кеше
		TTL: time.Minute,
	},
}

_, err := gashe.GetOrLoad(ctx, c, "user:999", users, load)
if errors.Is(err, sql.ErrNoRows) { // true и на первом вызове, и на закешированных
	...
}
```

**Область.** Отрицательный ответ - это сохраненная запись: с Redis он защищает базу от всех
процессов, с `memory` - только от этого.

**Пример.** 404 для несуществующего пользователя: парсер, перебирающий id, теперь бьет в кеш, а не в PostgreSQL. С pgx сравнивайте с `pgx.ErrNoRows`.

```go
var profiles = gashe.Policy[User]{
	TTL: time.Hour,
	Negative: gashe.Negative{
		If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
		Err: sql.ErrNoRows,
		TTL: 30 * time.Second,
	},
}

func (s *Service) ProfileHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := gashe.GetOrLoad(r.Context(), s.cache, "user:"+id, profiles, func(ctx context.Context) (User, error) {
		return s.repo.User(ctx, id) // возвращает sql.ErrNoRows для несуществующего id
	})
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(u)
}
```

> [!WARNING]
> Данные, созданные после того, как отрицательный ответ попал в кеш, остаются "не найдено" до
> конца `Negative.TTL`. Держите его коротким или делайте `Delete` ключа или инвалидацию его тега при создании
> данных.

### Нагрузка на источник

#### Cache stampede внутри процесса

**Проблема.** Ключа нет, и тысяча горутин спрашивает его в один момент; каждая вызывает загрузчик,
и база получает тысячу одинаковых запросов. Еще это называют dogpile effect.

**Решение.** Автоматически. Одновременные вызовы `GetOrLoad` с одним ключом и типом в одном
процессе делят одно чтение и один запуск загрузчика. Запуск использует контекст вызова, который его
начал; ждущий вызов возвращает свой `ctx.Err()`, если его контекст закончился раньше.

**Область.** Только этот процесс, с любым бэкендом. Между процессами - см. `LockOnMiss` ниже.

**Пример.** Страница товара: когда товар становится вирусным, тысячи одновременных запросов к нему выполняют один запрос в базу.

```go
func (s *Service) ProductHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := gashe.GetOrLoad(r.Context(), s.cache, "product:"+id, products, func(ctx context.Context) (Product, error) {
		return s.repo.Product(ctx, id) // выполняется один раз для всех одновременных запросов этого пода
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(p)
}
```

#### Cache stampede между процессами

**Проблема.** То же самое, но у вас пятьдесят подов: каждый под все равно один раз грузит
отсутствующий ключ, и база получает пятьдесят одинаковых запросов.

**Решение.** `LockOnMiss`: на промахе один процесс берет лок в бэкенде и выполняет загрузчик;
остальные ждут и затем читают его результат. Значение - сколько максимум держится лок, на случай,
если его владелец упадет. Лок берется только на промахе, никогда на попадании.

```go
var reports = gashe.Policy[Report]{TTL: time.Hour, LockOnMiss: 30 * time.Second}
```

**Область.** Все процессы, которые используют тот же Redis. С `memory` лок действует только в этом
процессе и ничего не добавляет к автоматическому схлопыванию выше.

**Пример.** Агрегат для дашборда по миллионам строк: на холодном кеше запрос выполняет один под, остальные его ждут.

```go
var stats = gashe.Policy[Stats]{TTL: 10 * time.Minute, LockOnMiss: time.Minute}

func (s *Service) Stats(ctx context.Context, day string) (Stats, error) {
	return gashe.GetOrLoad(ctx, s.cache, "stats:"+day, stats, func(ctx context.Context) (Stats, error) {
		var st Stats
		err := s.db.QueryRowContext(ctx,
			"SELECT count(*), coalesce(sum(amount), 0) FROM orders WHERE created_at::date = $1", day).
			Scan(&st.Orders, &st.Revenue)
		return st, err
	})
}
```

> [!WARNING]
> Делайте `LockOnMiss` дольше самой медленной загрузки, иначе по истечении лока загрузку начнет
> второй процесс. Ждущие вызовы ждут столько, сколько позволяет их `ctx`: давайте запросам дедлайн.

#### N+1 запросов на списках (пакетная загрузка)

**Проблема.** Страница показывает 50 товаров. Если грузить их по одному через `GetOrLoad`, это 50
обращений к кешу и, на холодном кеше, 50 запросов к базе.

**Решение.** `gashe.GetOrLoadMany` читает все ключи одним вызовом бэкенда и вызывает загрузчик один
раз с ключами, которых не было, чтобы он получил их одним запросом. То, что вернул загрузчик,
сохраняется одним вызовом; ключей, которые он не вернул, нет в результате, и они не сохраняются.

```go
products, err := gashe.GetOrLoadMany(ctx, c, ids, catalog,
	func(ctx context.Context, missing []string) (map[string]Product, error) {
		return s.repo.ProductsByID(ctx, missing) // найденные товары по id
	})
```

Он читает и пишет те же записи, что и `GetOrLoad` с той же политикой, поэтому оба могут отдавать
одни и те же ключи. Поддерживаются `TTL`, `TTLFunc`, `Jitter`, `Tags`, `CacheIf` и `Prefix`.

**Область.** Сохраненные результаты видят все процессы с Redis и только этот процесс с `memory`.
Сами вызовы не объединяются даже внутри одного процесса.

**Пример.** Страница корзины: товары корзины, один SQL-запрос для тех, которых нет в кеше.

```go
var catalog = gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.1, Prefix: "product:"}

func (s *Service) CartProducts(ctx context.Context, ids []string) (map[string]Product, error) {
	return gashe.GetOrLoadMany(ctx, s.cache, ids, catalog, func(ctx context.Context, missing []string) (map[string]Product, error) {
		found, err := s.repo.Products(ctx, missing) // SELECT ... WHERE id = ANY($1)
		if err != nil {
			return nil, err
		}
		out := make(map[string]Product, len(found))
		for _, p := range found {
			out[p.ID] = p
		}
		return out, nil
	})
}
```

> [!NOTE]
> `StaleWhileRevalidate`, `StaleIfError`, `LockOnMiss` и `Negative` работают по одному ключу, и с
> ними `GetOrLoadMany` возвращает `ErrInvalidOptions`; используйте политику без них. Одновременные
> вызовы тоже не объединяются: два запроса одного и того же холодного списка оба вызовут загрузчик.

#### Cache avalanche (разброс TTL)

**Проблема.** Много ключей записано одновременно (после деплоя, прогрева, пакетной задачи) и
поэтому истекает в одну и ту же секунду, и база получает все их загрузки разом.

**Решение.** `Jitter` случайно разбрасывает каждый TTL на долю до заданной.

```go
var products = gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.1} // от 54 до 66 минут
```

**Область.** Каждая сохраненная запись получает свой разброс, где бы она ни хранилась.

**Пример.** Прогрев каталога товаров при старте: без `Jitter` весь он истек бы в одну и ту же минуту через час.

```go
var catalog = gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.2}

func (s *Service) WarmUp(ctx context.Context, ids []string) {
	for _, id := range ids {
		_, err := gashe.Reload(ctx, s.cache, "product:"+id, catalog, func(ctx context.Context) (Product, error) {
			return s.repo.Product(ctx, id)
		})
		if err != nil {
			slog.WarnContext(ctx, "warm-up", "product", id, "error", err)
		}
	}
}
```

#### Горячие ключи и сетевые обращения (near cache)

**Проблема.** Один ключ читают десятки тысяч раз в секунду: каждое чтение - сетевое обращение, и
все они приходят на один узел Redis, где лежит ключ.

**Решение.** `nearcache` ставит кеш в памяти перед общим бэкендом. Чтения в течение `LocalTTL` идут из
памяти; записи и удаления идут в общий бэкенд и сбрасывают локальную копию. Локи, счетчики и TTL
всегда используют общий бэкенд.

```go
import "github.com/eugeneliukindev/gashe/backend/nearcache"

local := memory.New(memory.Options{MaxEntries: 10_000})
defer local.Close()
backend := nearcache.New(local, gasheredis.New(client), nearcache.Options{LocalTTL: 5 * time.Second})
c := gashe.New(backend, gashe.Options{})
```

Цена - устаревание: другой процесс может до `LocalTTL` отвечать значением, которое этот процесс уже
изменил, и инвалидация тега доходит до других процессов тоже в течение `LocalTTL`. Делайте его коротким.

**Область.** Локальный уровень у каждого процесса свой; общий уровень видят все процессы.

**Пример.** Feature flags, которые читаются на каждом запросе: с `nearcache` каждый под спрашивает о них Redis примерно раз в `LocalTTL`.

```go
var flags = gashe.Policy[Flags]{TTL: time.Minute}

func (s *Service) Flags(ctx context.Context) (Flags, error) {
	return gashe.GetOrLoad(ctx, s.cache, "flags", flags, func(ctx context.Context) (Flags, error) {
		var f Flags
		err := s.db.QueryRowContext(ctx, "SELECT new_checkout, dark_mode FROM flags").Scan(&f.NewCheckout, &f.DarkMode)
		return f, err
	})
}
```

> [!WARNING]
> Локальный уровень живет в каждом процессе: при 50 подах `MaxEntries` локального `memory.Backend`
> хранится 50 раз, и память каждого пода должна его вместить. Ограничьте его через `MaxBytes`, а не
> только числом записей, и держите `LocalTTL` коротким: это еще и то, сколько другие поды могут отвечать устаревшим
> значением.

### Хранение

#### Пересечение ключей в общем Redis

**Проблема.** Несколько сервисов или окружений делят один Redis, и их ключи пересекаются.

**Решение.** `gashe.WithPrefix` держит все ключи бэкенда под префиксом; `Keys` и `All` выдают ключи
без него, а `DeleteMatch` никогда не трогает чужие префиксы.

```go
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), "orders:prod:"), gashe.Options{})
```

**Пример.** Один Redis на несколько сервисов и окружений: у каждого сервиса свое пространство ключей.

```go
prefix := os.Getenv("SERVICE") + ":" + os.Getenv("ENV") + ":" // "orders:staging:"
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), prefix), gashe.Options{})
```

#### Большие значения

**Проблема.** Большие значения (длинные списки, отрендеренные страницы) занимают память в Redis и
сеть на каждом чтении.

**Решение.** `gashe.Gzip` сжимает то, что кодирует другой кодек.

```go
c := gashe.New(backend, gashe.Options{Codec: gashe.Gzip{Inner: gashe.JSON{}}})
```

Сжатое значение больше не простое число, поэтому `Incr` с ним не работает.

Еще два кодека уменьшают значения без сжатия:

- `github.com/eugeneliukindev/gashe/codec/msgpack` - отдельный модуль с `msgpack.Codec{}` на основе
  [vmihailenco/msgpack v5](https://github.com/vmihailenco/msgpack). MessagePack двоичный: он меньше
  JSON и быстрее декодируется. Он работает и внутри `Gzip`: `gashe.Gzip{Inner: msgpack.Codec{}}`.
- `gashe.Raw{}` хранит `[]byte` и `string` как есть, без кавычек и base64 из JSON, а остальные типы
  отдает в `Inner` (по умолчанию JSON). `GetOrLoad` хранит результат внутри своей записи, поэтому
  `Raw` помогает только `Get`, `Set` и другим базовым вызовам, например для уже отрендеренной
  страницы.

Значения в MessagePack тоже двоичные, поэтому `Incr` и с ними не работает.

**Пример.** Отрендеренная страница категории на несколько сотен килобайт хранится в сжатом виде.

```go
var pages = gashe.Policy[string]{TTL: 10 * time.Minute}

func (s *Service) CategoryPage(ctx context.Context, slug string) (string, error) {
	return gashe.GetOrLoad(ctx, s.pageCache, "page:category:"+slug, pages, func(ctx context.Context) (string, error) {
		return s.renderCategory(ctx, slug)
	})
}

// s.pageCache = gashe.New(backend, gashe.Options{Codec: gashe.Gzip{}})
```

> [!CAUTION]
> Смена кодека, как и смена `Prefix`, делает все сохраненные записи нечитаемыми: все они разом
> становятся промахами, и нагрузка уходит в базу. Выкатывайте такие изменения постепенно и держите
> `LockOnMiss` включенным для горячих ключей.

### Защита зависимостей

#### Взаимное исключение между процессами (распределенный лок)

**Проблема.** Какая-то работа никогда не должна выполняться дважды одновременно в ваших
процессах: дневной отчет, платеж, шаг миграции.

**Решение.** `Lock` и `TryLock` берут лок в бэкенде; он работает как `sync.Mutex`. Длительность
освобождает лок, если владелец упал; `Extend` продлевает его для работы, которая идет дольше
запланированного.

```go
lease, err := c.Lock(ctx, "report:daily", time.Minute) // ждет, пока лок у кого-то другого
if err != nil {
	return err
}
defer lease.Unlock(ctx)

for _, part := range parts {
	build(part)
	if err := lease.Extend(ctx, time.Minute); err != nil { // gashe.ErrLockLost, если истек
		return err
	}
}

lease, ok, err := c.TryLock(ctx, "report:daily", time.Minute) // не ждет
if !ok {
	return errBusy
}
```

- `Lock` проверяет занятый лок каждые `Options.LockPollInterval` (по умолчанию 100 мс) и
  останавливается с `ctx.Err()`, когда кончается `ctx`.
- `Unlock` освобождает лок, только если он все еще ваш, даже после отмены `ctx`.
- Если бэкенд упал, `Lock` и `TryLock` возвращают его ошибку: выполнять ли работу без лока - решать
  вам.

**Область.** Все процессы, которые используют тот же Redis. С `memory` лок действует только внутри
этого процесса.

**Пример.** Перестройка поискового индекса из cron, который запускается на каждом поде: перестраивает один под, остальные пропускают.

```go
func (s *Service) RebuildIndex(ctx context.Context) error {
	lease, ok, err := s.cache.TryLock(ctx, "job:reindex", time.Minute)
	if err != nil || !ok {
		return err // индекс перестраивает другой под, или Redis лежит
	}
	defer lease.Unlock(ctx)
	for offset := 0; ; offset += 1000 {
		n, err := s.indexBatch(ctx, offset, 1000)
		if err != nil || n == 0 {
			return err
		}
		if err := lease.Extend(ctx, time.Minute); err != nil {
			return err // лок потерян: остановиться, мог начать другой под
		}
	}
}
```

> [!CAUTION]
> Локи рекомендательные, см. [Известные ограничения](#известные-ограничения). Если лок защищает
> деньги или другие данные, где два писателя наносят вред, сделайте и саму запись безопасной для
> повтора (идемпотентной).

#### Перегрузка и злоупотребления (rate limiting)

**Проблема.** Клиент, пользователь или баг вызывает дорогой эндпоинт гораздо чаще, чем система
выдерживает.

**Решение.** `RateLimit` засчитывает вызов для ключа и возвращает `gashe.ErrRateLimited`, когда ключ
превысил лимит. Вызывайте его перед работой, которую он защищает.

```go
if err := c.RateLimit(ctx, "user:"+id, gashe.PerMinute(100)); err != nil {
	return err // gashe.ErrRateLimited
}

// Фиксированное окно с баном: 5 попыток входа в минуту, потом 15 минут тишины.
err := c.RateLimit(ctx, "login:"+ip, gashe.FixedWindow{Calls: 5, Per: time.Minute, BanFor: 15 * time.Minute})
```

- `gashe.SlidingWindow{Calls, Per}` и сокращения `PerSecond`, `PerMinute`, `PerHour`: около `Calls`
  вызовов за любой отрезок длиной `Per`. На ключ хранятся два счетчика, и считается, что вызовы
  прошлого отрезка шли равномерно. Отказанные вызовы не засчитываются.
- `gashe.FixedWindow{Calls, Per, BanFor}`: не больше `Calls` вызовов на отрезок `Per`, который начинается с
  первого вызова; первый вызов сверх лимита блокирует ключ на `BanFor` (по умолчанию `Per`).

**Область.** С Redis - один лимит на все процессы. С `memory` каждый процесс считает только свои
вызовы: при 10 подах настоящий лимит в 10 раз выше.

**Пример.** Middleware для HTTP: 600 запросов в минуту на API-ключ, дальше 429.

```go
func RateLimited(c *gashe.Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := "api-key:" + r.Header.Get("X-API-Key")
		if err := c.RateLimit(r.Context(), key, gashe.PerMinute(600)); errors.Is(err, gashe.ErrRateLimited) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

> [!NOTE]
> `SlidingWindow` - это оценка, и при всплесках трафика может пропустить чуть больше `Calls`. `FixedWindow`
> может пропустить до двух `Calls` около границы двух окон.

#### Каскадные отказы (circuit breaker)

**Проблема.** Зависимость начинает падать или отвечать по таймауту; каждый запрос все равно ждет
ее, горутины и соединения копятся, и отказ расползается на ваш сервис и его клиентов.

**Решение.** `CircuitBreaker` считает вызовы и ошибки за последнее `Window`. Когда ошибок
достаточно, он открывается: в течение `OpenFor` каждый вызов сразу получает `gashe.ErrCircuitOpen`.
Затем он переходит в состояние half-open: пропускает `HalfOpenCalls` пробных вызовов, первый
успешный закрывает его, неудачный снова открывает. Breaker живет в памяти `Cache`, поэтому его проверка не стоит сетевого обращения; каждый
процесс судит о зависимости по своим вызовам.

```go
var db = gashe.BreakerOptions{
	FailurePercent: 50,               // открыться, когда падает 50% вызовов
	MinCalls:       20,               // но только после хотя бы 20 вызовов
	Window:         time.Minute,      // считается за последнюю минуту
	OpenFor:        30 * time.Second, // оставаться открытым столько
	HalfOpenCalls:  3,                // потом пропустить 3 пробных вызова; 0 = просто закрыться после OpenFor
	IsFailure:      func(err error) bool { return !errors.Is(err, sql.ErrNoRows) }, // по умолчанию любая ошибка
}

done, err := c.CircuitBreaker("db", db)
if err != nil {
	return err // gashe.ErrCircuitOpen: даже не пытаться
}
rows, err := queryDB(ctx)
done(err) // сообщить, как прошло: nil или ошибка
```

Вызывайте `done` один раз. Вызов, о котором вы не сообщили, не учитывается. Для одного ключа
используйте одни и те же опции.

**Область.** У каждого процесса свой breaker, с любым бэкендом.

**Пример.** Вызовы платежного провайдера: пока он падает, оплата отвечает сразу, а не висит на таймаутах.

```go
var payments = gashe.BreakerOptions{
	FailurePercent: 50,
	MinCalls:       10,
	Window:         30 * time.Second,
	OpenFor:        15 * time.Second,
	HalfOpenCalls:  1,
}

func (s *Service) Charge(ctx context.Context, p Payment) error {
	done, err := s.cache.CircuitBreaker("payments-api", payments)
	if err != nil {
		return ErrPaymentsUnavailable // показать "попробуйте через минуту" вместо вечной загрузки
	}
	err = s.provider.Charge(ctx, p)
	done(err)
	return err
}
```

> [!WARNING]
> У каждого процесса свой breaker: при 50 подах каждый открывается после своих ошибок. Состояние
> каждого ключа хранится в памяти все время жизни `Cache`, поэтому используйте несколько постоянных
> ключей (`"db"`, `"payments-api"`), а не id пользователей или URL.

### Эксплуатация

#### Отказ кеша

**Проблема.** Redis лежит или тормозит, и сервис не должен падать вместе с ним.

**Решение.** Встроено. Если бэкенд упал, `GetOrLoad` вызывает загрузчик, возвращает его результат
без сохранения и отправляет ошибку кеша в `OnError`; `RateLimit` пропускает вызов. Сколько
медленный Redis может держать запрос, ограничивают таймауты go-redis (`ReadTimeout`,
`WriteTimeout`, см. [Redis](#redis)).

**Пример.** Redis лежит: запросы продолжают обслуживаться из базы, и вы видите это в логах.

```go
c := gashe.New(gasheredis.New(client), gashe.Options{
	OnError: func(ctx context.Context, err error) {
		slog.WarnContext(ctx, "cache unavailable, serving from the database", "error", err)
	},
})
```

> [!WARNING]
> Когда Redis лежит, вместе с ним пропадает и координация между процессами: `LockOnMiss` больше
> не сдерживает другие поды, и каждый под отправляет свои промахи в базу. На этот случай поставьте
> перед базой `CircuitBreaker`.

#### Доля попаданий и мониторинг

**Проблема.** Непонятно, помогает ли кеш: как часто он отвечает, как часто отдает старое значение
или обновляет.

**Решение.** `OnResult` получает, как был отвечен каждый вызов `GetOrLoad`.

**Область.** `OnResult` видит вызовы этого процесса; складывайте метрики всех подов.

```go
c := gashe.New(backend, gashe.Options{
	OnResult: func(ctx context.Context, key string, r gashe.Result) {
		cacheResults.WithLabelValues(r.String()).Inc() // hit, miss, stale-while-revalidate, stale-if-error, negative-hit
	},
	OnError: func(ctx context.Context, err error) {
		slog.WarnContext(ctx, "cache degraded", "error", err)
	},
})
```

> [!WARNING]
> `OnResult` и `OnError` выполняются синхронно на каждом вызове. Делайте их быстрыми: считайте, не
> пишите в сеть или в медленный лог на пути запроса.

#### Корректная остановка

**Проблема.** `StaleWhileRevalidate` обновляет запись в фоне. Когда сервис останавливается, такое
обновление может еще идти: оно обрывается на середине или пишет в уже закрытый бэкенд.

**Решение.** Вызовите `c.Shutdown(ctx)` перед закрытием бэкенда. Он перестает запускать фоновые
обновления и ждет уже запущенные, пока не завершится `ctx`. Кеш продолжает отвечать; устаревшие
результаты просто больше не обновляются.

**Область.** Фоновые обновления этого процесса.

**Пример.** Конец `main`: перестать принимать запросы, дать обновлениям закончиться, потом закрыть
Redis.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
_ = srv.Shutdown(ctx) // HTTP-сервер
if err := c.Shutdown(ctx); err != nil {
	slog.Warn("background reloads did not finish", "error", err)
}
_ = client.Close()
```

### Известные ограничения

- **Гонка cache-aside.** Чтение может загрузить из базы старое значение, запись может обновить
  базу и удалить ключ, а затем чтение сохранит свое старое значение; оно продержится до конца TTL.
  gashe это не предотвращает. Держите TTL часто меняющихся данных коротким или делайте
  инвалидацию тега после записи.
- **Локи рекомендательные.** Если владелец лока остановится дольше TTL лока (долгая пауза GC,
  замороженная виртуалка), другой процесс может взять лок, пока первый еще работает. `Extend`
  сообщает `gashe.ErrLockLost`, когда это замечает.
- **Часы.** О свежести решают часы читающего процесса относительно времени, которое записал
  пишущий. Держите часы синхронизированными (NTP).

## Инвалидация

Записи истекают по TTL. Чтобы изменить их раньше:

```go
gashe.Reload(ctx, c, "user:42", users, loadUser) // загрузить и сохранить новый результат сейчас
c.InvalidateTags(ctx, "user:42")                  // каждая запись с тегом становится промахом
c.Delete(ctx, "user:42")
c.DeleteMany(ctx, "user:1", "user:2")
c.DeleteMatch(ctx, "user:*")                      // обходит бэкенд: избегайте на большом Redis
```

Чтобы после изменения кода сбросить все записи одного вида, поменяйте `Prefix` в его опциях.

> [!CAUTION]
> Новый `Prefix` начинается с пустого кеша: все ключи разом промахиваются. Ожидайте нагрузку
> холодного кеша после такого деплоя.

### Инвалидация по тегам

**Проблема.** Одно изменение делает устаревшими много записей: изменился пользователь, а с ним его
профиль, страница заказов и настройки. Удаление по шаблону обходит весь бэкенд, а в Redis это
медленно.

**Решение.** `Tags` и `InvalidateTags`. У каждого тега есть счетчик поколения; запись при
сохранении запоминает поколения своих тегов, а `InvalidateTags` их увеличивает, и каждая запись с
этим тегом становится промахом. Инвалидация стоит одну запись на тег, а чтение берет запись и ее
поколения одним вызовом. Старые записи больше никогда не читаются и истекают по TTL.

```go
profile := gashe.Policy[Profile]{TTL: time.Hour, Tags: []string{"user:42"}}
orders := gashe.Policy[[]Order]{TTL: time.Hour, Tags: []string{"user:42", "orders"}}

p, err := gashe.GetOrLoad(ctx, c, "profile:42", profile, loadProfile)
o, err := gashe.GetOrLoad(ctx, c, "orders:42", orders, loadOrders)

c.InvalidateTags(ctx, "user:42") // теперь обе записи - промахи
c.InvalidateTags(ctx, "orders")  // теперь каждая запись заказов - промах
```

**Область.** С Redis инвалидацию сразу видят все процессы, через `nearcache` - в течение `LocalTTL`.
С `memory` - только этот процесс.

**Пример.** После UPDATE в PostgreSQL все закешированное о пользователе загружается заново: профиль, страница заказов, настройки.

```go
func (s *Service) Rename(ctx context.Context, userID int64, name string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = $2", name, userID); err != nil {
		return err
	}
	return s.cache.InvalidateTags(ctx, fmt.Sprint("user:", userID)) // после записи, а не до
}
```

> [!NOTE]
> Счетчики тегов никогда не истекают: каждое имя тега - это один маленький ключ навсегда.
> Теги на пользователя (`user:42`) - нормально, теги на запрос - нет. Каждый тег записи
> добавляет ключ к ее чтению.

## Ошибки

| Ошибка | Когда |
|---|---|
| `gashe.ErrNotFound` | `Get`, `TTL`, `Value.Decode` при промахе |
| `gashe.ErrInvalidTTL` | отрицательный TTL или неположительный TTL в `Expire`, `Lock`, `TryLock`, `Extend` |
| `gashe.ErrInvalidOptions` | `Policy`, лимит или `BreakerOptions`, которые не могут работать |
| `gashe.ErrWrongType` | в записи данные, с которыми операция не умеет работать, например `Incr` на JSON-объекте |
| `gashe.ErrLockLost` | `Extend` лока, который истек |
| `gashe.ErrRateLimited` | `RateLimit` отказал в вызове |
| `gashe.ErrCircuitOpen` | `CircuitBreaker` отказал в вызове |

Проверяйте их через `errors.Is`. Ошибки кодека оборачиваются один раз с операцией и ключом
(`gashe: decode "user:42": ...`). Ошибки бэкенда возвращаются в том виде, в котором их сообщил
бэкенд; бэкенд Redis оборачивает ошибки go-redis один раз, в виде `gashe/redis: get: dial tcp ...`.
Ошибки вашего загрузчика возвращаются без изменений, как и `Negative.Err`.

`GetOrLoad` и `RateLimit` не возвращают ошибки бэкенда: они продолжают работать без кеша. Эти
ошибки уходят в `Options.OnError`, обернутые операцией и ключом (`gashe: get "user:42": ...`).
`Lock`, `TryLock`, `Extend` и `InvalidateTags` ошибки бэкенда возвращают.

## Как тестировать свой код

Используйте бэкенд в памяти; отключите фоновую чистку, чтобы ни одна горутина не пережила тест:

```go
func newCache(t *testing.T) *gashe.Cache {
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	return gashe.New(mem, gashe.Options{})
}
```

TTL следуют `time.Now`, поэтому на Go 1.25+ тесты с `testing/synctest` идут в фиктивном времени
без настоящих `sleep`:

```go
synctest.Test(t, func(t *testing.T) {
	c := newCache(t)
	_ = c.Set(ctx, "k", 1, time.Hour)
	time.Sleep(time.Hour) // внутри пузыря мгновенно
	err := c.Get(ctx, "k", new(int)) // gashe.ErrNotFound
})
```

Фоновые перезагрузки выполняются в горутинах; внутри пузыря `synctest.Wait()` дает им завершиться.
`Jitter` делает TTL случайными: не указывайте его в тестах, которые проверяют точный TTL.

Чтобы изобразить упавший бэкенд, встройте настоящий и переопределите методы, которые хотите
сломать:

```go
type downBackend struct{ gashe.Backend }

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
```

## План

1. Ядро: `Backend`, бэкенд в памяти, базовое API, кеширование. Готово.
2. Бэкенд Redis в отдельном модуле. Готово.
3. Stale-while-revalidate, stale-if-error, лок от stampede, распределенные локи. Готово.
4. Rate limit и circuit breaker. Готово.
5. Один объект вокруг `Cache`, итераторы. Готово.
6. Отрицательное кеширование, разброс TTL, теги, `OnResult`, продление лока, breaker в памяти
   процесса, префиксы, gzip, двухуровневый near cache. Готово.
7. Пакетная загрузка многих ключей сразу (`GetOrLoadMany`), корректная остановка. Готово.
8. Мгновенная инвалидация near cache через Redis pub/sub.

## Разработка

В репозитории четыре модуля. Выполняйте каждую команду в каждом из них.

| Модуль | Что содержит | Зависимости |
|---|---|---|
| `.` | `gashe`, `gashe/backend/memory`, `gashe/backend/nearcache`, `gashe/backendtest` | нет |
| `backend/redis/` | `gashe/backend/redis` | go-redis v9 |
| `backend/redis/integration/` | тесты `gashe/backend/redis` на настоящем Redis | testcontainers-go, Docker |
| `codec/msgpack/` | `gashe/codec/msgpack` | vmihailenco/msgpack v5 |

Интеграционный модуль отдельный, чтобы testcontainers никогда не попал к пользователям
`gashe/backend/redis`. Его тесты сами поднимают `redis:7.0.0` в Docker и падают, если Docker недоступен.
Задайте `GASHE_REDIS_IMAGE`, чтобы прогнать их на другом сервере, например
`GASHE_REDIS_IMAGE=valkey/valkey:8 go test ./...`.

Модули поддерживают Go 1.23 и новее, сейчас это пять версий. Тесты, которым нужен
`testing/synctest`, стоят за `//go:build go1.25`, поэтому на Go 1.23 и 1.24 они пропускаются, а
остальные все равно выполняются. Интеграционному модулю нужен Go 1.25, как и testcontainers.

```bash
go test -race ./...  # Go 1.25+ выполняет все тесты, включая тесты в фиктивном времени
golangci-lint run    # golangci-lint v2, конфиг в .golangci.yml
GOENV_VERSION=1.23.12 GOTOOLCHAIN=local go test -race ./...  # самая старая поддерживаемая версия Go
```

`backend/redis/go.mod`, `backend/redis/integration/go.mod` и `codec/msgpack/go.mod` подменяют
модули gashe локальными копиями; перед публикацией уберите строки `replace` и укажите версии с тегами.

Термины предметной области описаны в `GLOSSARY.md`.
