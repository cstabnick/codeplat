// Package app builds llmrouter's parts from a config and serves them on
// one handler. It is the only place that chooses an implementation for a
// configured backend kind.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"llmrouter/internal/admin"
	"llmrouter/internal/config"
	"llmrouter/internal/gateway"
	"llmrouter/internal/llm"
	"llmrouter/internal/oaiclient"
	"llmrouter/internal/ollama"
	"llmrouter/internal/router"
	"llmrouter/internal/supervisor"
	"llmrouter/internal/ui"
)

// App is a running llmrouter: its fleet, router and HTTP handler.
type App struct {
	Config  config.Config
	Fleet   llm.Fleet
	Router  *router.Router
	Handler http.Handler

	sups    map[string]*supervisor.Supervisor
	clients map[string]*ollama.Client
	logger  *log.Logger
}

// New builds every part from cfg. It starts nothing. It logs to logger,
// which must not be nil.
func New(cfg config.Config, logger *log.Logger) (*App, error) {
	a := &App{
		Config:  cfg,
		sups:    make(map[string]*supervisor.Supervisor, len(cfg.Backends)),
		clients: make(map[string]*ollama.Client),
		logger:  logger,
	}
	backends := make([]llm.Backend, 0, len(cfg.Backends))
	classifiers := make(map[string]router.Classifier, len(cfg.Backends))
	for _, bc := range cfg.Backends {
		var b llm.Backend
		switch bc.Kind {
		case llm.KindOllama:
			c := ollama.New(bc.Name, bc.URL, nil)
			a.clients[bc.Name] = c
			b = c
		case llm.KindOpenAI:
			b = oaiclient.New(bc.Name, bc.URL, nil)
		default:
			return nil, fmt.Errorf("app: backend %q: unknown kind %q", bc.Name, bc.Kind)
		}
		a.sups[bc.Name] = supervisor.New(supervisor.Options{
			Name:         bc.Name,
			Health:       func(ctx context.Context) error { _, err := b.Health(ctx); return err },
			Command:      bc.Command,
			Dir:          bc.Dir,
			Env:          bc.Env,
			StopCommand:  bc.StopCommand,
			StartTimeout: time.Duration(bc.StartTimeoutSeconds) * time.Second,
			Log:          logger,
		})
		backends = append(backends, b)
		classifiers[bc.Name] = b.(router.Classifier)
	}

	sups := a.sups
	a.Fleet = llm.NewFleet(backends, func(name string) bool {
		s, ok := sups[name]
		return ok && s.Managed()
	})

	rt, err := router.New(classifiers, router.Options{
		IntentBackend: cfg.Intent.Backend,
		IntentModel:   cfg.Intent.Model,
		Routes:        cfg.Routes,
		Default:       cfg.DefaultRoute,
		Timeout:       time.Duration(cfg.Intent.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	a.Router = rt

	mux := http.NewServeMux()
	mux.Handle("/v1/", gateway.NewHandler(a.Fleet, rt, logger))
	mux.Handle("/api/", admin.NewHandler(a.Fleet, rt))
	mux.Handle("/", ui.NewHandler(a.Fleet, rt))
	a.Handler = mux
	return a, nil
}

// Start starts each backend that does not answer and has a command,
// logging (not returning) failures. For each kind-ollama backend with
// PullMissing whose supervisor's Start returned nil (the backend
// answers, whether or not Start just launched it), it pulls cfg.ModelsFor(name) in the
// background with supervisor.EnsureModels, logging the result. The
// returned channel is closed when every background pull has finished.
func (a *App) Start(ctx context.Context) <-chan struct{} {
	var wg sync.WaitGroup
	for _, bc := range a.Config.Backends {
		if err := a.sups[bc.Name].Start(ctx); err != nil {
			a.logger.Printf("%s: %v", bc.Name, err)
			continue
		}
		if !bc.PullMissing {
			continue
		}
		c, ok := a.clients[bc.Name]
		if !ok {
			continue
		}
		models := a.Config.ModelsFor(bc.Name)
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := supervisor.EnsureModels(ctx, c, models); err != nil {
				a.logger.Printf("%s: pull models: %v", name, err)
				return
			}
			a.logger.Printf("%s: models ready: %v", name, models)
		}(bc.Name)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	return done
}

// Close stops every backend process Start started, joining errors.
func (a *App) Close() error {
	var errs []error
	for _, bc := range a.Config.Backends {
		if err := a.sups[bc.Name].Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
