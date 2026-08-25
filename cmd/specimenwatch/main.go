package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/httpapi"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

func main() {
	var address, dataDir string
	var selfcheck bool
	flag.StringVar(&address, "addr", "", "回环 HTTP 监听地址")
	flag.StringVar(&dataDir, "data", "data", "本地持久化目录")
	flag.BoolVar(&selfcheck, "selfcheck", false, "运行回环 HTTP 冒烟后退出")
	flag.Parse()
	resolved, err := resolveAddress(address)
	if err != nil {
		log.Fatal(err)
	}
	if selfcheck {
		if err := runSelfcheck(resolved); err != nil {
			log.Fatal(err)
		}
		log.Print("selfcheck passed")
		return
	}
	if err := run(resolved, dataDir); err != nil {
		log.Fatal(err)
	}
}

func buildHandler(dataDir string) (http.Handler, error) {
	repo, err := store.Open(dataDir)
	if err != nil {
		return nil, err
	}
	calculator := assessment.New(assessment.DefaultRules())
	service := workflow.New(repo, calculator)
	return httpapi.NewHandler(service), nil
}

func run(address, dataDir string) error {
	handler, err := buildHandler(dataDir)
	if err != nil {
		return err
	}
	server := httpapi.NewServer(address, handler)
	listener, err := httpapi.Listen(server)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", address, err)
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	log.Printf("SpecimenTransitGuard listening on %s", listener.Addr())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Printf("收到信号 %s，正在停止", sig)
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}
