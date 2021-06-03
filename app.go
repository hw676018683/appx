package appx

import (
	"context"
	"fmt"
)

const (
	stateInstalling = iota + 1
	stateInstalled
	stateUninstalled
)

type InitCleaner interface {
	// Init initializes an application with the given context ctx.
	// It will return an error if fails.
	Init(Context) error

	// Clean does the cleanup work for an application. It will return an error if fails.
	Clean() error
}

type StartStopper interface {
	// Start kicks off a long-running application, like network servers or
	// message queue consumers. It will return an error if fails.
	Start(context.Context) error

	// Stop gracefully stops a long-running application. It will return an
	// error if fails.
	Stop(context.Context) error
}

type Validator interface {
	Validate() error
}

// Context is a set of context parameters used to initialize an application.
type Context struct {
	context.Context
	App       *App
	Required  map[string]*App
	Lifecycle Lifecycle
}

// InitFuncV2 initializes an application with the given context ctx.
// It will return an error if fails.
type InitFuncV2 func(ctx Context) error

// InitFunc initializes an application with the given context ctx, lifecycle lc
// and the required applications apps. When successful, It will return a value
// and a cleanup function that associated with the initialized application.
// Otherwise, it will return an error.
type InitFunc func(ctx context.Context, lc Lifecycle, apps map[string]*App) (Value, CleanFunc, error)

type OldInitFunc func(ctx context.Context, apps map[string]*App) (Value, CleanFunc, error)

// CleanFunc does the cleanup work for an application. It will return an error if fails.
type CleanFunc func() error

// Value is the value of an application, which is use-case specific and should
// be customized by users.
type Value interface{}

// App is a modular application.
type App struct {
	Name  string
	Value Value

	requiredNames map[string]bool
	requiredApps  map[string]*App
	getAppFunc    func(name string) (*App, error) // The function used to find an application by its name.

	instance InitCleaner // The user-defined application instance.

	initFunc   InitFunc
	initFuncV2 InitFuncV2
	cleanFunc  CleanFunc

	state int // The installation state.
}

// New creates an application with the given name.
func New(name string) *App {
	return &App{
		Name:          name,
		requiredNames: make(map[string]bool),
		requiredApps:  make(map[string]*App),
		getAppFunc: func(name string) (*App, error) {
			return nil, fmt.Errorf("app %q is not registered", name)
		},
	}
}

// New creates an application with the given name.
func NewV2(name string, instance InitCleaner) *App {
	return &App{
		Name:          name,
		requiredNames: make(map[string]bool),
		requiredApps:  make(map[string]*App),
		getAppFunc: func(name string) (*App, error) {
			return nil, fmt.Errorf("app %q is not registered", name)
		},
		instance:   instance,
		initFuncV2: instance.Init,
		cleanFunc:  instance.Clean,
	}
}

// Require sets the names of the applications that the current application requires.
func (a *App) Require(names ...string) *App {
	for _, name := range names {
		a.requiredNames[name] = true
	}
	return a
}

// Instance returns the underlying user-defined application instance.
func (a *App) Instance() interface{} {
	return a.instance
}

// Init sets the function used to initialize the current application.
// Init is deprecated in favor of Init2.
func (a *App) Init(initFunc OldInitFunc) *App {
	a.initFunc = func(ctx context.Context, lc Lifecycle, apps map[string]*App) (Value, CleanFunc, error) {
		return initFunc(ctx, apps)
	}
	return a
}

// InitV2 sets the function used to initialize the current application.
func (a *App) InitV2(initFunc InitFunc) *App {
	a.initFunc = initFunc
	return a
}

// InitFunc sets the function used to initialize the current application.
func (a *App) InitFunc(initFuncV2 InitFuncV2) *App {
	a.initFuncV2 = initFuncV2
	return a
}

// CleanFunc sets the function used to clean up the current application.
func (a *App) CleanFunc(cleanFunc CleanFunc) *App {
	a.cleanFunc = cleanFunc
	return a
}

// Install does the initialization work for the current application.
func (a *App) Install(ctx context.Context, lc Lifecycle, after func(*App)) (err error) {
	switch a.state {
	case stateInstalled:
		return nil // Do nothing since the application has already been installed.
	case stateInstalling:
		return fmt.Errorf("circular dependency is detected for app %q", a.Name)
	}

	// Mark the state as `installing`.
	a.state = stateInstalling

	// Install all the required applications.
	if err := a.prepareRequiredApps(); err != nil {
		return err
	}
	for _, app := range a.requiredApps {
		if err = app.Install(ctx, lc, after); err != nil {
			return err
		}
	}

	if a.instance != nil {
		/////////////////////////////////////////////////////
		// New logic for cases where app is created by NewV2.

		// Unmarshal possible configurations into the app instance.
		unmarshal := config.unmarshaller()
		if err := unmarshal(ctx, a.Name, a.instance); err != nil {
			return err
		}

		// Install the app instance.
		if err := a.instance.Init(Context{
			Context:  ctx,
			App:      a,
			Required: a.requiredApps,
		}); err != nil {
			return err
		}

		// If a.instance implements StartStopper, set the appropriate
		// lifecycle hooks.
		if startStopper, ok := a.instance.(StartStopper); ok {
			lc.Append(Hook{
				OnStart: startStopper.Start,
				OnStop:  startStopper.Stop,
			})
		}

		// If a.instance implements Validator, trigger the validation.
		if validator, ok := a.instance.(Validator); ok {
			if err := validator.Validate(); err != nil {
				return err
			}
		}
	} else {
		///////////////////////////////////////////////////
		// Old logic for cases where app is created by New.

		// Finally install the app itself.
		if a.initFuncV2 != nil {
			if err := a.initFuncV2(Context{
				Context:   ctx,
				App:       a,
				Required:  a.requiredApps,
				Lifecycle: lc,
			}); err != nil {
				return err
			}
		}

		// Finally install the app itself.
		if a.initFunc != nil {
			a.Value, a.cleanFunc, err = a.initFunc(ctx, lc, a.requiredApps)
			if err != nil {
				return err
			}
		}
	}

	if after != nil {
		// Call the hook function after installed, if any.
		after(a)
	}

	a.state = stateInstalled
	return nil
}

// Uninstall does the cleanup work for the current application.
func (a *App) Uninstall() (err error) {
	if a.state == stateUninstalled {
		return nil
	}

	if a.instance != nil {
		/////////////////////////////////////////////////////
		// New logic for cases where app is created by NewV2.

		if err = a.instance.Clean(); err != nil {
			return err
		}
	} else {
		///////////////////////////////////////////////////
		// Old logic for cases where app is created by New.

		if a.cleanFunc != nil {
			if err = a.cleanFunc(); err != nil {
				return err
			}
		}
	}

	a.state = stateUninstalled
	return nil
}

// prepareRequiredApps sets the field a.requiredApps of app if it's not set.
func (a *App) prepareRequiredApps() error {
	if len(a.requiredNames) == len(a.requiredApps) {
		return nil
	}

	for name := range a.requiredNames {
		app, err := a.getAppFunc(name)
		if err != nil {
			return err
		}
		a.requiredApps[name] = app
	}

	return nil
}
