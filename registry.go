package appx

import (
	"context"
	"fmt"
)

var (
	// registry holds all the registered applications.
	registry = make(map[string]*App)
)

// Register registers the application app into the registry.
func Register(app *App) error {
	if app == nil {
		return fmt.Errorf("nil app %v", app)
	}

	if app.Name == "" {
		return fmt.Errorf("the name of app %v is empty", app)
	}

	if _, ok := registry[app.Name]; ok {
		return fmt.Errorf("app %q is already registered", app.Name)
	}

	registry[app.Name] = app
	app.getAppFunc = getApp // Find an application in the registry.
	return nil
}

// MustRegister is like Register but panics if there is an error.
func MustRegister(app *App) {
	if err := Register(app); err != nil {
		panic(err)
	}
}

// Install installs the applications specified by names, with the given ctx.
// If no name is specified, all registered applications will be installed.
func Install(ctx context.Context, names ...string) error {
	if len(names) == 0 {
		for _, app := range registry {
			if err := app.Install(ctx); err != nil {
				return err
			}
		}
	}

	for _, name := range names {
		app, err := getApp(name)
		if err != nil {
			return err
		}
		if err := app.Install(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Uninstall uninstalls the applications specified by names.
// If no name is specified, all registered applications will be uninstalled.
func Uninstall(names ...string) error {
	if len(names) == 0 {
		for _, app := range registry {
			if err := app.Uninstall(); err != nil {
				return err
			}
		}
	}

	for _, name := range names {
		app, err := getApp(name)
		if err != nil {
			return err
		}
		if err := app.Uninstall(); err != nil {
			return err
		}
	}

	return nil
}

func getApp(name string) (*App, error) {
	app, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("app %q is not registered", name)
	}

	return app, nil
}
