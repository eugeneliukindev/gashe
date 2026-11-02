# gashe

[English](README.md) | Русский

Кеширование для Go-сервисов: сохраняет результаты дорогих вызовов, продолжает отвечать, когда
зависимость тормозит или лежит, и защищает зависимости локами, ограничением частоты (rate limit)
и circuit breaker. Все это на одном объекте `*gashe.Cache`.

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/redis # бэкенд Redis, зависит от go-redis v9
```

Нужен Go 1.23 или новее.

## Зачем

Кеш - это не только `Get` и `Set`. Реальному сервису нужно продолжать отвечать, пока лежит база,
обновлять популярный ключ так, чтобы никто не ждал, не давать холодному ключу отправить в базу сотню
одинаковых запросов разом и не падать, когда сломался сам кеш. gashe дает этим приемам простые
названия и собирает их вокруг одного объекта, так что в коде остается обычный вызов функции.

## Возможности

- Один объект: локи, лимиты, circuit breaker и базовое API - методы `*gashe.Cache`; типизированное
  кеширование - `gashe.GetOrLoad(ctx, c, key, opts, load)`, оно возвращает ваш тип.
- Старые результаты, когда вы это разрешили: обновление в фоне или ответ последним удачным
  результатом, если загрузка упала, и все это в одной записи кеша.
- Лок на промахе: холодный ключ грузит один процесс, остальные ждут его результат.
- Одновременные вызовы с одним ключом делят одно чтение и одну загрузку.
- Распределенные локи, скользящий и фиксированный лимиты, circuit breaker с пробными вызовами.
- Хранилища за одним интерфейсом: LRU в памяти с TTL и Redis (один узел, Sentinel или Cluster)
  в отдельном модуле.
- Значения любого типа через сменный `Codec` (по умолчанию JSON).
- Если сломался сам кеш, ваш код продолжает работать, а ошибка уходит в ваш обработчик `OnError`.
- Без глобального состояния и без сторонних зависимостей в основном модуле; Go от 1.23 до самой
  новой версии.

## Пример

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/memory"
)

type User struct {
	ID   int
	Name string
}

// users описывает, как кешировать пользователей; задается один раз.
var users = gashe.CacheOptions[User]{
	TTL:            time.Hour,      // свежий в течение часа
	FallbackWindow: 24 * time.Hour, // если база лежит, отвечать старым пользователем до суток
}

func loadUser(ctx context.Context, id int) (User, error) {
	return User{ID: id, Name: "Ann"}, nil // в реальном коде - запрос в базу
}

func main() {
	ctx := context.Background()

	mem := memory.New(memory.Options{Size: 10_000})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// Кеширование: из кеша, если там есть, иначе loadUser.
	u, err := gashe.GetOrLoad(ctx, c, "user:42", users, func(ctx context.Context) (User, error) {
		return loadUser(ctx, 42)
	})
	fmt.Println(u, err)

	// Лимит перед дорогим вызовом.
	if err := c.RateLimit(ctx, "user:42", gashe.PerMinute(100)); err != nil {
		fmt.Println(err) // gashe.ErrRateLimited
	}

	// Базовое API.
	_ = c.Set(ctx, "key", 90, 2*time.Hour)
	var n int
	if err := c.Get(ctx, "key", &n); errors.Is(err, gashe.ErrNotFound) {
		fmt.Println("miss")
	}
}
```

## Содержание

- [Настройка](#настройка)
- [Бэкенды](#бэкенды)
- [Базовое API](#базовое-api)
- [Стратегии](#стратегии)
  - [GetOrLoad: кеширование](#getorload-кеширование)
  - [Локи](#локи)
  - [Rate limit](#rate-limit)
  - [Circuit breaker](#circuit-breaker)
  - [Как их сочетать](#как-их-сочетать)
- [Ключи](#ключи)
- [TTL](#ttl)
- [Что можно кешировать](#что-можно-кешировать)
- [Инвалидация](#инвалидация)
- [Ошибки](#ошибки)
- [Как тестировать свой код](#как-тестировать-свой-код)
- [План](#план)
- [Разработка](#разработка)

## Настройка

Все настраивается структурами опций. Поле, которое вы не указали, получает значение по умолчанию,
поэтому пустая структура всегда работает.

```go
mem := memory.New(memory.Options{
	Size:          10_000,      // хранить не больше стольких записей; 0 значит 1000
	CheckInterval: time.Minute, // как часто выбрасывать истекшие записи; 0 значит раз в секунду
})
defer mem.Close()

c := gashe.New(mem, gashe.Options{
	Codec:   gashe.JSON{}, // как значения превращаются в байты; не указывайте для JSON
	OnError: onError,      // см. ниже
})
```

| Поле `gashe.Options` | Для чего |
|---|---|
| `Codec` | Превращает ваши значения в байты для бэкенда и обратно. JSON, если не задан другой. |
| `OnError` | `GetOrLoad`, `RateLimit` и `CircuitBreaker` никогда не падают только из-за того, что сломался кеш: они продолжают работать без него. Ошибка кеша не теряется, она приходит в эту функцию. Используйте ее, чтобы логировать или считать такие ошибки. Без нее они отбрасываются. |

```go
onError := func(ctx context.Context, err error) {
	slog.WarnContext(ctx, "cache degraded", "error", err)
}
```

Создайте один `Cache` при старте и используйте его везде; он безопасен для одновременного
использования. Бэкенд он не закрывает: закрывайте бэкенд там, где создали.

## Бэкенды

### В памяти

`github.com/eugeneliukindev/gashe/memory` хранит записи в памяти процесса: LRU фиксированного
размера, где у каждой записи может быть свой TTL. Истекшие записи никогда не возвращаются; они
удаляются при чтении и фоновой чисткой каждые `CheckInterval`. Байты копируются при записи и при
чтении, поэтому изменение значения после `Set` или `Get` никогда не меняет кеш.

Шаблоны в `Keys`, `All` и `DeleteMatch` используют синтаксис glob из Redis: `*`, `?`, `[abc]`,
`[^a]`, `[a-z]` и `\` для экранирования.

### Redis

`github.com/eugeneliukindev/gashe/redis` - отдельный модуль поверх
[go-redis v9](https://github.com/redis/go-redis), поэтому основной модуль остается без зависимостей.
Клиент go-redis вы создаете сами (адрес, TLS, пул, Sentinel или Cluster - это опции go-redis) и сами
его закрываете: бэкенд им не владеет.

```go
import (
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/redis"
)

client := goredis.NewUniversalClient(&goredis.UniversalOptions{Addrs: []string{"localhost:6379"}})
defer client.Close()
c := gashe.New(gasheredis.New(client), gashe.Options{})
```

Заметки:

- В Cluster `Keys`, `All`, `DeleteMatch`, `KeysCount` и `Clear` обходят все master-узлы, а операции
  с несколькими ключами отправляются по одной команде на ключ, поэтому ключи могут лежать в разных
  слотах.
- `Keys` и `All` могут выдать ключ больше одного раза, как это делает Redis `SCAN`.
- `Clear` выполняет `FLUSHDB`: удаляет все ключи выбранной базы, а не только записи gashe.
- `Size` возвращает `MEMORY USAGE`, туда входят накладные расходы Redis.
- Ошибки Redis оборачиваются один раз, в виде `gashe/redis: <op>: <error>`. Отсутствующий ключ
  становится `gashe.ErrNotFound`; ответ `WRONGTYPE` или нецелое значение в `Incr` -
  `gashe.ErrWrongType`.

### Свой бэкенд

Реализуйте `gashe.Backend`. Он работает только с байтами: `Cache` кодирует значения до того, как
они попадут в бэкенд. Точный контракт описан в комментариях к методам в `backend.go`: `ttl`, равный
0, означает "без срока", промах - это `gashe.ErrNotFound`, а каждый метод должен быть безопасен для
одновременного вызова.

Проверьте свой бэкенд тем же набором тестов контракта, который проходят встроенные бэкенды:

```go
func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // новый пустой бэкенд для каждого подтеста
	})
}
```

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
c.Size(ctx, "key")                          // (int64, error); байты, 0 для отсутствующего ключа
c.KeysCount(ctx)                            // (int64, error)
c.Ping(ctx)                                 // error
c.Clear(ctx)                                // error

c.Backend()                                 // gashe.Backend, для работы с сырыми байтами
```

Целое, записанное через `Incr`, читается через `Get` в `int`, а JSON-число, записанное через `Set`,
можно увеличивать через `Incr`.

## Стратегии

| Вам нужно | Используйте |
|---|---|
| кешировать результаты | `gashe.GetOrLoad` с `TTL` |
| продолжать отвечать, когда лежит база или API | `GetOrLoad` с `FallbackWindow` |
| чтобы пользователи никогда не ждали медленной перезагрузки популярного ключа | `GetOrLoad` с `RefreshWindow` |
| чтобы отсутствующий ключ грузил один процесс, а не все сразу | `GetOrLoad` с `LockOnMiss` |
| чтобы результат загружался заново после N чтений | `GetOrLoad` с `MaxReads` |
| чтобы задача никогда не выполнялась дважды одновременно, во всех процессах | `c.Lock` / `c.TryLock` |
| ограничить, как часто ключ может вызывать | `c.RateLimit` |
| перестать вызывать зависимость, которая все время падает | `c.CircuitBreaker` |

### GetOrLoad: кеширование

`gashe.GetOrLoad` возвращает результат, сохраненный под ключом. Если его нет или он слишком старый,
вызывает вашу функцию (**загрузчик**), сохраняет результат и возвращает его.

```go
var users = gashe.CacheOptions[User]{TTL: time.Hour}

u, err := gashe.GetOrLoad(ctx, c, "user:42", users, func(ctx context.Context) (User, error) {
	return db.LoadUser(ctx, 42) // выполняется, только когда кеш не может ответить
})
```

- `"user:42"` - это ключ. Вы сами собираете его из аргументов.
- `users` - это `CacheOptions[User]`: правила для этого вида данных. Задайте один раз и используйте
  повторно.
- Загрузчик возвращает `User`, значит и `GetOrLoad` возвращает `User`. Это функция, а не метод `c`,
  потому что до Go 1.27 у методов не может быть параметров типа.
- Ошибка загрузчика возвращается вам как есть и никогда не сохраняется.
- Десять одновременных вызовов с одним ключом в одном процессе вызывают загрузчик один раз и делят
  результат.
- Если сломался сам кеш (Redis лежит), загрузчик выполняется, вы получаете его результат, а ошибка
  кеша уходит в `OnError`.
- Опции, которые не могут работать (например, отрицательная длительность), возвращают
  `gashe.ErrInvalidOptions`.

Сохраненный результат проходит до трех стадий:

```
  сохранен         TTL кончился             TTL + окно кончилось
    |----- свежий -----|-------- старый --------|   удален
       ответ из кеша        ответ только в
                            разрешенных случаях
```

- **Свежий**: ответ из кеша, загрузчик не вызывается.
- **Старый**: без дополнительных опций игнорируется, и выполняется загрузчик. `RefreshWindow` и
  `FallbackWindow` говорят, когда старый результат еще годится.
- **Удален**: стерт из кеша; следующий вызов выполняет загрузчик.

#### Опции

**`TTL`** - сколько результат остается свежим.

```go
gashe.CacheOptions[User]{TTL: time.Hour} // обычный кеш: час, потом загрузить заново
```

**`FallbackWindow`** - сколько после `TTL` старый результат отвечает вместо упавшего загрузчика.
Нужно, чтобы продолжать работать, пока лежит база или API. Сначала все равно выполняется загрузчик;
только если он вернул ошибку, вы получаете старый результат (и nil вместо ошибки). Ошибка
загрузчика уходит в `OnError`.

```go
// Свежий 5 минут. Потом загружаем заново; если API упал, отвечаем старым
// результатом до часа.
gashe.CacheOptions[Rates]{TTL: 5 * time.Minute, FallbackWindow: time.Hour}
```

**`FallbackIf`** - какие ошибки загрузчика покрывает `FallbackWindow`. Без него - любые. Нужно,
например, чтобы "не найдено" возвращалось ошибкой, а покрывалось только "API лежит".

```go
FallbackIf: func(err error) bool { return errors.Is(err, ErrAPIDown) },
```

**`RefreshWindow`** - сколько после `TTL` старый результат отдается сразу, пока кеш перезагружает
его в фоне. Нужно для популярных ключей: никто не ждет медленного загрузчика, и перезагрузку
выполняет только один вызывающий во всех ваших процессах. Неудачная перезагрузка оставляет старый
результат, а ошибка уходит в `OnError`.

```go
// Свежий 10 минут. Следующие 2 минуты вызывающие сразу получают старый результат, а один
// из них перезагружает его в фоне.
gashe.CacheOptions[Feed]{TTL: 10 * time.Minute, RefreshWindow: 2 * time.Minute}
```

**`LockOnMiss`** - когда ключа в кеше нет совсем, загрузчик выполняет только один процесс,
остальные ждут и берут его результат. Нужно, когда холодный ключ иначе отправил бы в базу сотни
одинаковых запросов разом. Значение - сколько максимум держится лок, на случай, если процесс,
который его держит, упадет. Лок берется только на промахе, никогда на попадании.

```go
gashe.CacheOptions[Report]{TTL: time.Hour, LockOnMiss: 30 * time.Second}
```

**`CacheIf`** - решает, стоит ли сохранять результат. Без него сохраняется каждый успешный
результат. `gashe.NotNil` пропускает nil-указатели, слайсы и map.

```go
CacheIf: gashe.NotNil[*User], // не кешировать "нет такого пользователя"
```

**`MinLoadTime`** - сохранять результат, только если загрузчик работал дольше этого. Быстрые ответы
не стоят места в кеше.

```go
MinLoadTime: 100 * time.Millisecond,
```

**`MaxReads`** и **`RefreshAfterReads`** - загрузить результат заново после того, как его прочитали
`MaxReads` раз, даже если он еще свежий. На чтении номер `RefreshAfterReads` он перезагружается в
фоне, продолжая отдаваться. При этом каждое чтение обновляет счетчик, то есть чтение стоит записи.
`MaxReads` требует `TTL`.

```go
gashe.CacheOptions[Status]{TTL: 2 * time.Hour, MaxReads: 100, RefreshAfterReads: 90}
```

**`TTLFunc`** - выбирать `TTL` по результату, а не одно фиксированное значение. Верните
отрицательную длительность, чтобы не сохранять этот результат.

```go
TTLFunc: func(u User) time.Duration {
	if u.Admin {
		return time.Minute // админы меняются часто
	}
	return time.Hour
},
```

**`Prefix`** - добавляется перед каждым ключом (`v2:user:42`). Меняйте его, когда меняется форма
вашего типа, чтобы старые записи не читались в новый тип.

Опции можно сочетать:

```go
// Свежий 4 минуты, потом минута обновления в фоне, а если API лежит - старый результат
// до часа. Все это одна запись в кеше.
gashe.CacheOptions[Status]{
	TTL:            4 * time.Minute,
	RefreshWindow:  time.Minute,
	FallbackWindow: time.Hour,
}
```

`TTL` может быть 0, если задано окно:

- только `FallbackWindow: 2 * time.Hour`: загрузчик выполняется на каждом вызове, а последний
  удачный результат отвечает на его ошибки до двух часов (кеш "на случай отказа");
- только `RefreshWindow: 24 * time.Hour`: каждый вызов сразу получает сохраненный результат и
  запускает перезагрузку в фоне; ключ, который никто не читал сутки, истекает.

#### Reload

Чтобы заменить сохраненный результат прямо сейчас, например после обновления пользователя:

```go
u, err := gashe.Reload(ctx, c, "user:42", users, loadUser) // вызывает загрузчик и сохраняет результат
```

Служебные ключи: фоновая перезагрузка держит `<key>:refresh` максимум минуту, `LockOnMiss` держит
`<key>:load`, а `MaxReads` считает в `<key>:reads`.

### Локи

Лок гарантирует, что какая-то работа никогда не выполняется дважды одновременно для одного ключа,
во всех процессах, которые делят бэкенд. Работает как `sync.Mutex`: взять, отпустить через `defer`.

```go
unlock, err := c.Lock(ctx, "report:daily", time.Minute) // ждет, пока лок у кого-то другого
if err != nil {
	return err
}
defer unlock(ctx)

unlock, ok, err := c.TryLock(ctx, "report:daily", time.Minute) // не ждет
if !ok {
	return errBusy // отчет уже строит кто-то другой
}

locked, err := c.IsLocked(ctx, "report:daily")
```

- Длительность освобождает лок на случай, если его владелец упал; делайте ее больше времени работы.
- `Lock` повторяет попытку каждые 100 мс и останавливается с `ctx.Err()`, когда кончается `ctx`.
- `unlock` освобождает лок, только если он все еще ваш, и работает даже после отмены `ctx`.

### Rate limit

`RateLimit` засчитывает вызов для ключа и возвращает `gashe.ErrRateLimited`, когда ключ превысил
лимит. Вызывайте его перед работой, которую он защищает; лимит общий для всех процессов.

```go
if err := c.RateLimit(ctx, "user:"+id, gashe.PerMinute(100)); err != nil {
	return err // gashe.ErrRateLimited
}
```

Два вида лимитов:

- **`gashe.Sliding{Calls, Per}`**, или сокращения `gashe.PerSecond(n)`, `gashe.PerMinute(n)`,
  `gashe.PerHour(n)`: около `Calls` вызовов за любой отрезок длиной `Per`. Это оценка: на ключ
  хранятся два счетчика, и считается, что вызовы прошлого отрезка шли равномерно. Отказанные вызовы
  не засчитываются.
- **`gashe.Fixed{Calls, Per, BanFor}`**: не больше `Calls` вызовов на отрезок `Per`, который
  начинается с первого вызова. Первый вызов сверх лимита блокирует ключ на `BanFor` (по умолчанию
  `Per`), считая от этого вызова.

```go
c.RateLimit(ctx, "login:"+ip, gashe.Fixed{Calls: 5, Per: time.Minute, BanFor: 15 * time.Minute})
```

Если бэкенд упал, вызов пропускается, а ошибка уходит в `OnError`.

### Circuit breaker

Перестает вызывать то, что все время падает: зависимость получает время восстановиться, а ваши
вызывающие сразу получают ответ вместо ожидания таймаутов.

Работает в два шага: спросить перед вызовом, сообщить результат после.

```go
var db = gashe.BreakerOptions{
	FailurePercent: 50,               // открыться, когда падает 50% вызовов
	MinCalls:       20,               // но только после хотя бы 20 вызовов
	Window:         time.Minute,      // считается за последнюю минуту
	OpenFor:        30 * time.Second, // оставаться открытым столько
	TrialCalls:     3,                // потом пропустить 3 пробных вызова; 0 = просто закрыться после OpenFor
	IsFailure:      func(err error) bool { return !errors.Is(err, sql.ErrNoRows) }, // по умолчанию любая ошибка
}

done, err := c.CircuitBreaker(ctx, "db", db)
if err != nil {
	return err // gashe.ErrCircuitOpen: даже не пытаться
}
rows, err := queryDB(ctx)
done(err) // сообщить, как прошло: nil или ошибка
```

- Пока breaker открыт, каждый вызов получает `gashe.ErrCircuitOpen`, ваш код не выполняется.
- После `OpenFor` он пропускает `TrialCalls` вызовов: первый успешный закрывает breaker, неудачный
  снова открывает. Если за `Window` пробный вызов так и не пришел, breaker закрывается сам.
- Вызывайте `done` ровно один раз. Вызов, о котором вы не сообщили, просто не учитывается.
- Если бэкенд упал, breaker считается закрытым, а ошибка уходит в `OnError`.

### Как их сочетать

```go
// Ответ из кеша; на промахе спросить лимит и breaker перед вызовом API.
// Если API упал или защита отказала, отвечать последним удачным статусом до часа.
var statuses = gashe.CacheOptions[Status]{TTL: time.Minute, FallbackWindow: time.Hour}

st, err := gashe.GetOrLoad(ctx, c, "status:"+id, statuses, func(ctx context.Context) (Status, error) {
	if err := c.RateLimit(ctx, "api", gashe.PerSecond(50)); err != nil {
		return Status{}, err
	}
	done, err := c.CircuitBreaker(ctx, "api", apiBreaker)
	if err != nil {
		return Status{}, err
	}
	st, err := api.Status(ctx, id)
	done(err)
	return st, err
})
```

## Ключи

Ключи - это строки, которые вы собираете в коде из аргументов вызова:

```go
gashe.GetOrLoad(ctx, c, fmt.Sprintf("user:%d:lang:%s", id, lang), users, load)
gashe.GetOrLoad(ctx, c, fmt.Sprintf("token:%x", sha256.Sum256([]byte(token))), tokens, load)
```

`GetOrLoad` добавляет `Prefix` как `prefix:key`. Лимиты и circuit breaker хранят данные под своими
префиксами (`srl:`, `rate_limit:`, `circuit_breaker:`), поэтому никогда не пересекаются с вашими
записями. Для разных типов значений нужны разные ключи: чтение ключа как другого типа - это ошибка
декодирования, которую `GetOrLoad` считает промахом и загружает заново. Когда меняется форма
кешируемого типа, поменяйте его `Prefix` (`"v2"`), чтобы старые записи не читались в новый тип.

## TTL

TTL - обычные значения `time.Duration`.

- В `GetOrLoad` `TTL` - это сколько результат свежий, а запись хранится `TTL` плюс более длинное из
  окон. `TTLFunc` выбирает TTL для каждого результата. См. [опции](#опции).
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

## Инвалидация

Записи истекают по TTL. Чтобы изменить их раньше:

```go
gashe.Reload(ctx, c, "user:42", users, loadUser) // загрузить и сохранить новый результат сейчас
c.Delete(ctx, "user:42")
c.DeleteMany(ctx, "user:1", "user:2")
c.DeleteMatch(ctx, "user:*")
```

Чтобы после изменения кода сбросить все записи одного вида, поменяйте `Prefix` в его опциях.

## Ошибки

| Ошибка | Когда |
|---|---|
| `gashe.ErrNotFound` | `Get`, `TTL`, `Value.Decode` при промахе |
| `gashe.ErrInvalidTTL` | отрицательный TTL или неположительный TTL в `Expire`, `Lock`, `TryLock` |
| `gashe.ErrInvalidOptions` | `CacheOptions`, лимит или `BreakerOptions`, которые не могут работать |
| `gashe.ErrWrongType` | в записи данные, с которыми операция не умеет работать, например `Incr` на JSON-объекте |
| `gashe.ErrRateLimited` | `RateLimit` отказал в вызове |
| `gashe.ErrCircuitOpen` | `CircuitBreaker` отказал в вызове |

Проверяйте их через `errors.Is`. Ошибки кодека оборачиваются один раз с операцией и ключом
(`gashe: decode "user:42": ...`). Ошибки бэкенда возвращаются в том виде, в котором их сообщил
бэкенд; бэкенд Redis оборачивает ошибки go-redis один раз, в виде `gashe/redis: get: dial tcp ...`.
Ошибки вашего загрузчика возвращаются без изменений.

`GetOrLoad`, `RateLimit` и `CircuitBreaker` не возвращают ошибки бэкенда: они продолжают работать
без кеша. Эти ошибки уходят в `Options.OnError`, обернутые операцией и ключом
(`gashe: get "user:42": ...`), чтобы вы могли их логировать или считать. Без `OnError` они
отбрасываются. `Lock` и `TryLock` ошибки бэкенда возвращают: выполнять ли работу без лока - решать
вам.

## Как тестировать свой код

Используйте бэкенд в памяти; отключите фоновую чистку, чтобы ни одна горутина не пережила тест:

```go
func newCache(t *testing.T) *gashe.Cache {
	mem := memory.New(memory.Options{CheckInterval: -1})
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

Чтобы изобразить упавший бэкенд, встройте настоящий и переопределите методы, которые хотите
сломать:

```go
type downBackend struct{ gashe.Backend }

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
```

## План

1. Ядро: `Backend`, бэкенд в памяти, базовое API, кеширование. Готово.
2. Бэкенд Redis в отдельном модуле. Готово.
3. Старые результаты (обновление в фоне, ответ при ошибке), лок на промахе, лимит чтений, локи.
   Готово.
4. Rate limit и circuit breaker. Готово.
5. Один объект: защиты и базовое API - методы `Cache`, кеширование - `gashe.GetOrLoad`; итераторы
   для ключей. Готово.
6. Теги.
7. Хуки наблюдения, двухуровневый бэкенд (память перед Redis), разброс TTL, кеширование
   результатов "не найдено".
8. Пакетная загрузка многих ключей сразу.
9. Кодек со сжатием, middleware для net/http, метрики Prometheus.

## Разработка

В репозитории три модуля. Выполняйте каждую команду в каждом из них.

| Модуль | Что содержит | Зависимости |
|---|---|---|
| `.` | `gashe`, `gashe/memory`, `gashe/backendtest` | нет |
| `redis/` | `gashe/redis` | go-redis v9 |
| `redis/integration/` | тесты `gashe/redis` на настоящем Redis | testcontainers-go, Docker |

Интеграционный модуль отдельный, чтобы testcontainers никогда не попал к пользователям
`gashe/redis`. Его тесты сами поднимают `redis:7.0.0` в Docker и падают, если Docker недоступен.

Модули поддерживают Go 1.23 и новее, сейчас это пять версий. Тесты, которым нужен
`testing/synctest`, стоят за `//go:build go1.25`, поэтому на Go 1.23 и 1.24 они пропускаются, а
остальные все равно выполняются. Интеграционному модулю нужен Go 1.25, как и testcontainers.

```bash
go test -race ./...  # Go 1.25+ выполняет все тесты, включая тесты в фиктивном времени
golangci-lint run    # golangci-lint v2, конфиг в .golangci.yml
GOENV_VERSION=1.23.12 GOTOOLCHAIN=local go test -race ./...  # самая старая поддерживаемая версия Go
```

`redis/go.mod` и `redis/integration/go.mod` подменяют модули gashe локальными копиями; перед
публикацией модуля Redis уберите строки `replace` и укажите версии с тегами.

Решения по устройству записаны в `docs/adr`, термины предметной области - в `GLOSSARY.md`.
