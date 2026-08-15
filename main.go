package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dsc/plugin"
)

func main() {
	// 1. 創建插件管理器
	mgr := plugin.NewManager(&plugin.ManagerConfig{
		PluginDir: "./plugins/",
		Handshake: plugin.Handshake,
	})
	defer mgr.Shutdown()

	// 2. 加載插件
	if err := mgr.Load("example", "./plugins/example/example"); err != nil {
		log.Printf("Failed to load plugin: %v", err)
	}

	// 3. 調用插件
	if p, ok := mgr.Get("example"); ok {
		ctx := context.Background()
		resp, err := p.Execute(ctx, &plugin.ExecuteRequest{
			Input:  "Hello DSC",
			Params: map[string]string{"key": "value"},
		})
		if err != nil {
			log.Printf("Execute error: %v", err)
		} else {
			fmt.Printf("Response: %+v\n", resp)
		}

		fmt.Printf("Plugin Name: %s\n", p.Name(ctx))
		fmt.Printf("Plugin Version: %s\n", p.Version(ctx))
	}

	// 4. 熱重載示例
	go func() {
		time.Sleep(5 * time.Second)
		fmt.Println("\n[Main] Hot-reloading plugin...")
		if err := mgr.HotReload("example", "./plugins/example/example_v2"); err != nil {
			log.Printf("Hot-reload failed: %v", err)
		}
	}()

	// 5. 等待退出信號
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\n[Main] Shutting down...")
}