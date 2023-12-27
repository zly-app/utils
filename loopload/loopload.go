package loopload

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/zly-app/zapp/component/metrics"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"
	"github.com/zly-app/zapp/pkg/utils"
	"github.com/zlyuancn/zutils"
	"go.uber.org/zap"
)

type loadFunc[T any] func() (T, error)

type LoopLoad[T any] struct {
	name        string
	metricsName string
	value       *zutils.AtomicValue[T]
	loadFn      loadFunc[T]
	opts        *options

	done       chan struct{}
	startState int32 // 0=未启动, 1=已启动
	loadState  int32 // 0=未加载, 1=加载成功, 2=再次加载中
}

func New[T any](name string, loadFn loadFunc[T], opts ...Option) *LoopLoad[T] {
	metricsName := "loopload_" + name
	metrics.RegistryCounter(metricsName, "", metrics.Labels{"name": name}, "")
	initV := new(T)
	ret := &LoopLoad[T]{
		name:        name,
		metricsName: metricsName,
		value:       zutils.NewAtomic[T](*initV),
		loadFn:      loadFn,
		opts:        newOptions(opts),

		done:       make(chan struct{}, 0),
		startState: 0,
		loadState:  0,
	}

	handler.AddHandler(handler.BeforeStartHandler, func(app core.IApp, handlerType handler.HandlerType) {
		err := ret.start(app)
		if err != nil {
			app.Fatal("loopload start err", zap.String("name", "name"), zap.Error(err))
		}
	})
	handler.AddHandler(handler.BeforeExitHandler, func(app core.IApp, handlerType handler.HandlerType) {
		ret.close()
	})
	return ret
}

// 启动
func (l *LoopLoad[T]) start(app core.IApp) error {
	if !atomic.CompareAndSwapInt32(&l.loadState, 0, 2) {
		return errors.New("loadState!=0")
	}
	defer atomic.StoreInt32(&l.loadState, 1)

	// 立即加载
	err := l.load()
	if err != nil {
		return err
	}

	app.GetComponent().GetGPool().Go(func() error {
		t := time.NewTicker(l.opts.reloadTime)
		for {
			select {
			case <-t.C:
				_ = l.Load()
			case <-l.done:
				t.Stop()
				l.done <- struct{}{}
				return nil
			}
		}
	}, nil)

	return nil
}
func (l *LoopLoad[T]) close() {
	if !atomic.CompareAndSwapInt32(&l.startState, 1, 0) {
		return
	}

	l.done <- struct{}{}
	<-l.done
}

// 立即加载
func (l *LoopLoad[T]) Load() error {
	if !atomic.CompareAndSwapInt32(&l.loadState, 1, 2) {
		return nil
	}
	defer atomic.StoreInt32(&l.loadState, 1)

	return l.load()
}

func (l *LoopLoad[T]) load() error {
	var ret T
	err := utils.Recover.WrapCall(func() error {
		res, err := l.loadFn()
		ret = res
		return err
	})
	if err != nil {
		return err
	}
	l.value.Set(ret)
	return nil
}

func (l *LoopLoad[T]) Get() T {
	return l.value.Get()
}
