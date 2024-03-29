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

	l := loopload.New[*int]("test", func(ctx context.Context) (*int, error) {
		fmt.Println("reload")
		v := 1
		return &v, nil
	}, loopload.WithReloadTime(time.Second*10))
	go func() {
		time.Sleep(1e9)
		a := l.Get(context.Background())
		fmt.Println("数据", *a)
	}()
	app.Run()
}
