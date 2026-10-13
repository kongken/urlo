// urlo-migrate is a deployment tool, deliberately separate from Web startup.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, getenv func(string) string, out, diagnostic io.Writer) error {
	flags := flag.NewFlagSet("urlo-migrate", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	env := flags.String("dsn-env", "URLO_POSTGRES_DSN", "environment variable containing the migration-role DSN")
	timeout := flags.Duration("timeout", 5*time.Minute, "total deadline including advisory-lock wait (maximum 1h)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments; supply the DSN through the environment")
	}
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(*env) {
		return errors.New("invalid DSN environment variable name")
	}
	if *timeout <= 0 || *timeout > time.Hour {
		return errors.New("timeout must be greater than zero and at most 1h")
	}
	dsn := getenv(*env)
	if dsn == "" {
		return fmt.Errorf("environment variable %s is required", *env)
	}
	ctx, cancel := context.WithTimeout(parent, *timeout)
	defer cancel()
	result, err := migrate(ctx, dsn)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "migrations complete: applied=%d version=%d\n", result.Applied, result.Version)
	return err
}
