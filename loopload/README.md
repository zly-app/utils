# loopload

基于 [zapp](https://github.com/zly-app/zapp) 框架的泛型周期加载器，用于定期从数据源加载/刷新数据，并提供线程安全的读取接口。

## 特性

- 泛型支持：基于 Go 1.18+ 泛型，可加载任意类型数据
- 周期刷新：可配置的定时重新加载间隔（默认 1 分钟）
- 线程安全：基于原子操作的状态管理，并发安全
- 生命周期集成：自动绑定 zapp 的 `BeforeStart` / `BeforeExit` 生命周期
- Filter 链支持：`Load` 和 `Get` 操作均经过 zapp client filter 链，可接入链路追踪、指标采集等
- 手动加载：支持通过 `Load()` 方法主动触发立即刷新

## 安装

```bash
go get github.com/zly-app/utils/loopload
```

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/zly-app/zapp"

    "github.com/zly-app/utils/loopload"
)

func main() {
    app := zapp.NewApp("test")
    defer app.Exit()

    // 创建一个周期加载器，每 10 秒重新加载一次
    l := loopload.New[*int]("test", func(ctx context.Context) (*int, error) {
        fmt.Println("reload")
        v := 1
        return &v, nil
    }, loopload.WithReloadTime(time.Second*10))

    go func() {
        time.Sleep(time.Second)
        a := l.Get(context.Background())
        fmt.Println("数据", *a)
    }()

    app.Run()
}
```

## API

### `New[T any](name string, loadFn func(ctx context.Context) (T, error), opts ...Option) *LoopLoad[T]`

创建 LoopLoad 实例。创建后会自动注册到 zapp 生命周期：

- 应用启动前（`BeforeStart`）：立即执行首次加载，并启动定时刷新协程
- 应用退出前（`BeforeExit`）：优雅停止定时刷新协程

**参数：**

| 参数 | 说明 |
|------|------|
| `name` | 加载器名称，用于日志和 filter 链标识 |
| `loadFn` | 数据加载函数，返回加载的数据或错误 |
| `opts` | 可选配置项 |

### `Get(ctx context.Context) T`

获取当前已加载的数据。线程安全，可并发调用。

### `Load(ctx context.Context) error`

主动触发一次数据重新加载。仅在正常状态下生效，如果正在加载中则跳过。

## 配置项

### `WithReloadTime(t time.Duration) Option`

设置定时重新加载间隔，默认为 `1 * time.Minute`。

## 状态流转

```
0(未加载) --start()--> 2(首次加载中) --加载成功--> 1(正常) --close()--> 3(已停止)
                                           ↑          |
                                           +--Load()--+
                                              (重新加载中，完成回到1)
```

| 状态值 | 含义 |
|--------|------|
| 0 | 未加载，等待 start |
| 1 | 正常运行 |
| 2 | 加载中（首次加载或重新加载） |
| 3 | 已停止 |

## 设计说明

- **首次加载失败**：`start()` 返回错误，应用将 Fatal 退出
- **定时刷新失败**：仅记录错误，不影响当前已加载的数据，下个周期会再次尝试
- **并发安全**：`Get()` 使用原子值读取，`Load()` 使用 CAS 确保同时只有一个加载操作执行
- **Filter 链**：`Load` 和 `Get` 均经过 zapp 的 client filter，filter 标识为 `loopload/{name}/Load` 和 `loopload/{name}/Get`

## AI 使用说明

如需了解本库的详细架构和 AI 辅助开发指南，请参阅 [AI 说明文件](./.ai/loopload.md)。
