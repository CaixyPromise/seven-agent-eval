package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"seven-agent-eval/internal/core"
	"seven-agent-eval/internal/server"
	"syscall"
	"time"
)

func printJSON(v any) { b, _ := json.MarshalIndent(v, "", "  "); fmt.Println(string(b)) }
func homeDefault() string {
	if p := os.Getenv("SEVEN_EVAL_HOME"); p != "" {
		return p
	}
	candidates := []string{}
	if exe, e := os.Executable(); e == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	if wd, e := os.Getwd(); e == nil {
		candidates = append(candidates, wd)
	}
	for _, start := range candidates {
		for p := start; ; p = filepath.Dir(p) {
			if _, e := os.Stat(filepath.Join(p, "sdk/python/pyproject.toml")); e == nil {
				return p
			}
			if filepath.Dir(p) == p {
				break
			}
		}
	}
	return "."
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "seven-eval:", e)
		os.Exit(1)
	}
}
func run() error {
	fs := flag.NewFlagSet("seven-eval", flag.ContinueOnError)
	pkg := fs.String("package", ".", "Evaluation Package root")
	home := fs.String("home", homeDefault(), "Seven Eval installation directory")
	state := fs.String("state", "", "state directory (default: package/.seven-evaluate)")
	addr := fs.String("addr", "127.0.0.1:7431", "local server address")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	args := fs.Args()
	if len(args) == 0 {
		return errors.New("usage: seven-eval [--package DIR] [--json] init|discover|plan inspect|run|serve|export|verify|retention")
	}
	// --json is also accepted after read-only subcommands.
	cleaned := []string{}
	for _, arg := range args {
		if arg == "--json" {
			*asJSON = true
		} else {
			cleaned = append(cleaned, arg)
		}
	}
	args = cleaned
	cmd := args[0]
	if cmd == "init" {
		if len(args) != 2 {
			return errors.New("usage: init DIRECTORY")
		}
		if e := core.InitPackage(args[1]); e != nil {
			return e
		}
		fmt.Println("Created evaluation package:", args[1])
		return nil
	}
	if cmd == "verify" {
		if len(args) != 2 {
			return errors.New("usage: verify FILE.seven.eval")
		}
		r, e := core.ImportFile(args[1])
		if e != nil {
			return e
		}
		printJSON(map[string]any{"valid": true, "run_id": r.ID, "status": r.Status})
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	root, err := core.FindPackage(*pkg)
	viewing := cmd == "serve" && len(args) == 2
	if err != nil {
		if !viewing && cmd != "export" {
			return err
		}
		root, err = filepath.Abs(*pkg)
		if err != nil {
			return err
		}
	}
	worker := core.NewWorker(*home)
	if cmd == "discover" || cmd == "plan" {
		d, e := core.Discover(ctx, root, worker)
		if e != nil {
			return e
		}
		if cmd == "plan" {
			if len(args) != 3 || args[1] != "inspect" {
				return errors.New("usage: plan inspect PLAN")
			}
			effective, e := core.Resolve(root, d, args[2])
			if e != nil {
				return e
			}
			printJSON(effective)
			return nil
		}
		if *asJSON {
			printJSON(d)
		} else {
			fmt.Printf("%s@%s · %s\n", d.Package.ID, d.Package.Version, d.Package.Name)
			for _, a := range d.Assets {
				fmt.Printf("%-9s %-12s %-25s %s %s\n", a.Status, a.Kind, a.ID, a.Path, a.Message)
			}
		}
		return nil
	}
	if *state == "" {
		*state = filepath.Join(root, ".seven-evaluate")
	}
	store, err := core.NewStore(*state)
	if err != nil {
		return err
	}
	defer store.Close()
	engine := &core.Engine{Root: root, Worker: worker, Store: store}
	switch cmd {
	case "retention":
		if len(args) != 3 {
			return errors.New("usage: retention preview RFC3339-CUTOFF | retention apply PREVIEW-DIGEST")
		}
		live, e := core.NewLiveManager(engine)
		if e != nil {
			return e
		}
		defer live.Close()
		switch args[1] {
		case "preview":
			before, e := time.Parse(time.RFC3339, args[2])
			if e != nil {
				return errors.New("cutoff must be RFC3339 with timezone")
			}
			preview, e := live.PreviewRetention(before)
			if e != nil {
				return e
			}
			if e = live.SaveRetentionPreview(preview); e != nil {
				return e
			}
			printJSON(preview)
			return nil
		case "apply":
			audit, e := live.ApplyRetention(ctx, args[2])
			if e != nil {
				return e
			}
			printJSON(audit)
			return nil
		default:
			return errors.New("unknown retention operation")
		}
	case "run":
		if len(args) != 2 {
			return errors.New("usage: run PLAN")
		}
		r, e := engine.Prepare(ctx, args[1])
		if e != nil {
			return e
		}
		r, e = engine.Execute(ctx, r)
		if e != nil {
			return e
		}
		dest := filepath.Join(*state, "exports", r.ID+".seven.eval")
		if e = core.Export(r, dest); e != nil {
			return e
		}
		printJSON(map[string]any{"run_id": r.ID, "status": r.Status, "completed": r.Completed, "total": r.Total, "bundle": dest})
		if r.Status != "completed" {
			return errors.New("run incomplete; inspect retained report")
		}
		return nil
	case "export":
		if len(args) != 3 {
			return errors.New("usage: export RUN_ID FILE.seven.eval")
		}
		r, e := store.Get(args[1])
		if e != nil {
			return e
		}
		return core.Export(r, args[2])
	case "serve":
		if len(args) > 2 {
			return errors.New("usage: serve [FILE.seven.eval]")
		}
		if viewing {
			r, e := core.ImportFile(args[1])
			if e != nil {
				return e
			}
			if existing, e := store.Get(r.ID); e == nil {
				a, ae := existing.CaseDigest()
				b, be := r.CaseDigest()
				if ae != nil || be != nil || a != b || core.Digest(existing) != core.Digest(r) {
					return errors.New("run id collision")
				}
			} else if _, e = store.ImportRun(r); e != nil {
				return e
			}
			fmt.Printf("Opened %s\n", r.ID)
		}
		if err = server.ListenAddress(*addr); err != nil {
			return err
		}
		web := filepath.Join(*home, "web", "dist")
		if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
			return errors.New("web UI not built; run make build")
		}
		app := server.New(engine, web)
		httpServer := &http.Server{Addr: *addr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		done := make(chan error, 1)
		go func() { done <- httpServer.ListenAndServe() }()
		fmt.Printf("Seven Eval: http://%s\n", *addr)
		select {
		case e := <-done:
			app.Shutdown()
			if !errors.Is(e, http.ErrServerClosed) {
				return e
			}
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdown)
			app.Shutdown()
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}
