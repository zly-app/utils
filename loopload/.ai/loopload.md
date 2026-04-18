# loopload - AI 辅助开发说明

> 本文件面向 AI 编程助手，提供该库的架构细节、设计决策和修改指南。

## 模块概览

`loopload` 是一个基于 zapp 框架的泛型周期数据加载器，核心功能是定期从数据源加载数据并缓存，提供线程安全的读取接口。

## 文件结构

```
loopload/
├── loopload.go   # 核心实现：LoopLoad[T] 结构体、创建/启动/关闭/加载/获取
├── opts.go       # 配置选项：options 结构体、Option 函数类型、WithReloadTime
├── example/
│   └── main.go   # 使用示例
├── .ai/
│   └── loopload.md  # 本文件
└── README.md     # 人类可读的说明文档
```

## 核心类型

### `LoopLoad[T any]`

泛型结构体，承载周期加载的所有状态：

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | `string` | 加载器名称，用于日志和 filter 链路由 |
| `value` | `*zutils.AtomicValue[T]` | 原子值容器，存储当前已加载数据 |
| `loadFn` | `loadFunc[T]` | 用户提供的加载函数签名：`func(ctx context.Context) (T, error)` |
| `opts` | `*options` | 配置项，当前仅 `reloadTime` |
| `done` | `chan struct{}` | 关闭信号通道，用于优雅停止定时协程 |
| `loadState` | `int32` | 原子状态，控制加载生命周期（见状态机） |

### `options`

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `reloadTime` | `time.Duration` | `time.Minute` | 定时刷新间隔 |

## 状态机

`loadState` 使用原子操作（`atomic.CompareAndSwapInt32`）管理，状态转换如下：

```
0 (未加载)
  │
  ├─ New() 创建时初始化为 0
  │
  └─ start() CAS(0→2) ──→ 2 (首次加载中)
                              │
                              ├─ 加载失败: Store(0) 回退到未加载
                              └─ 加载成功: Store(1) ──→ 1 (正常运行)
                                                       │
                                  ┌─────────────────────┤
                                  │                     │
                            Load() CAS(1→2)       close() CAS(1→3 或 2→3)
                                  │                     │
                                  └─ defer Store(1)     └─→ 3 (已停止)
                                  │
                                  └─→ 2 (重新加载中)
                                       │
                                       └─ defer Store(1) → 1 (正常运行)
```

**关键设计决策：**

- `close()` 中有多次 CAS 尝试（1→3, 2→3, 再试 1→3），是因为加载完成后状态从 2 变为 1 存在时间窗口，需要重试确保在并发场景下能正确关闭。
- `Load()` 仅在状态 1 时可触发（CAS 1→2），其他状态下调用直接返回 nil（不报错），避免与 start/close 冲突。

## 生命周期集成

通过 zapp 的 handler 机制绑定：

1. **`New()` 中注册** `handler.BeforeStartHandler`：调用 `start()`，执行首次加载并启动定时协程
2. **`New()` 中注册** `handler.BeforeExitHandler`：调用 `close()`，停止定时协程

这意味着 **LoopLoad 必须在 zapp 应用上下文中使用**，不能独立运行。

## Filter 链集成

`load()` 和 `Get()` 均通过 `filter.GetClientFilter` 创建 filter 链：

- **load**: `filter.GetClientFilter(ctx, "loopload", l.name, "Load")`
  - filter 标识：`loopload/{name}/Load`
  - 请求为 nil，响应为加载的数据
- **Get**: `filter.GetClientFilter(ctx, "loopload", l.name, "Get")`
  - filter 标识：`loopload/{name}/Get`
  - 请求为 nil，响应为当前缓存数据

此外，`load()` 内部使用 `utils.Recover.WrapCall` 包裹，确保加载函数的 panic 不会导致协程崩溃。

## 修改指南

### 添加新的配置选项

1. 在 `opts.go` 的 `options` 结构体中添加字段
2. 在 `newOptions()` 中设置默认值
3. 添加对应的 `With*` Option 函数
4. 在 `loopload.go` 中使用新配置

### 修改加载逻辑

核心加载逻辑在 `load()` 方法中，注意：

- 保持 `utils.Recover.WrapCall` 包裹以防止 panic
- 保持 filter 链调用以维持可观测性
- 加载成功后必须调用 `l.value.Set(ret)` 更新缓存

### 修改状态机

修改 `loadState` 相关逻辑时需格外注意并发安全：

- 所有状态变更必须使用原子操作
- CAS 操作的失败路径必须正确处理
- 考虑 start/close/load 三个操作的并发场景

## 依赖关系

- `github.com/zly-app/zapp` — 应用框架，提供生命周期、filter、协程池等
- `github.com/zlyuancn/zutils` — 提供 `AtomicValue[T]` 原子值容器
- `go.uber.org/zap` — 结构化日志

## 注意事项

- 本库强依赖 zapp 框架，无法独立使用
- 泛型要求 Go 1.18+
- `Get()` 返回的是值拷贝（对于值类型）或指针（对于指针类型），修改返回值不会影响缓存
- 定时刷新协程运行在 zapp 的协程池中，不是独立 goroutine
