package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

func newCmdOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()

	u, err := ui.New(ui.Options{Stdout: stdout, Stderr: stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func newCmdRuntimeOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return newCmdRuntimeIOContext(t, strings.NewReader(""), stdout, stderr)
}

func newCmdRuntimeIOContext(t *testing.T, stdin io.Reader, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return app.WithRuntime(newCmdOutputContext(t, stdout, stderr), &app.Runtime{IO: app.IO{
		In:  stdin,
		Out: stdout,
		Err: stderr,
	}})
}

func newCmdJSONContext(t *testing.T) context.Context {
	t.Helper()
	return newCmdJSONOutputContext(t, io.Discard, io.Discard)
}

func newCmdJSONOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return outfmt.WithMode(newCmdOutputContext(t, stdout, stderr), outfmt.Mode{JSON: true})
}

func newCmdRuntimeJSONOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return outfmt.WithMode(newCmdRuntimeOutputContext(t, stdout, stderr), outfmt.Mode{JSON: true})
}

func withTestRuntime(ctx context.Context, configure func(*app.Runtime)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	configure(runtime)
	return app.WithRuntime(ctx, runtime)
}

func withAuthStore(ctx context.Context, store secrets.Store) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Auth.OpenSecretsStore = func() (secrets.Store, error) {
			return store, nil
		}
	})
}

func withAuthOperations(ctx context.Context, operations app.AuthOperations) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Auth = operations
	})
}

func runtimeWithAuthStore(store secrets.Store) *app.Runtime {
	return &app.Runtime{Auth: app.AuthOperations{
		OpenSecretsStore: func() (secrets.Store, error) {
			return store, nil
		},
	}}
}

func rootFlagsWithAuthStore(flags *RootFlags, store secrets.Store) *RootFlags {
	if flags == nil {
		flags = &RootFlags{}
	} else {
		clone := *flags
		flags = &clone
	}
	flags.authOperations = runtimeWithAuthStore(store).Auth
	return flags
}

func executeWithPeopleDirectoryTestService(t *testing.T, args []string, svc *people.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		PeopleDirectory: func(context.Context, string) (*people.Service, error) {
			return svc, nil
		},
	}})
}

type executeTestResult struct {
	stdout string
	stderr string
	err    error
}

func executeWithTestRuntime(t *testing.T, args []string, runtime *app.Runtime) executeTestResult {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if runtime == nil {
		runtime = &app.Runtime{}
	} else {
		runtimeCopy := *runtime
		runtime = &runtimeCopy
	}
	runtime.IO = app.IO{
		In:  strings.NewReader(""),
		Out: &stdout,
		Err: &stderr,
	}
	err := executeWithRuntime(args, runtime)
	return executeTestResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}
