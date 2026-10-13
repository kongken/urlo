package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCommandValidationDoesNotConnectOrExposeDSN(t *testing.T) {
	for _, args := range [][]string{
		{"--timeout=0s"}, {"--timeout=2h"}, {"--dsn-env=invalid-name"}, {"unexpected"},
	} {
		var output bytes.Buffer
		called := false
		err := run(t.Context(), args, func(string) string { called = true; return "secret" }, &output, &output)
		if err == nil || called {
			t.Fatalf("args=%v error=%v env read=%v", args, err, called)
		}
	}
	for _, dsn := range []string{"", "postgres://user:secret-password@%bad/db"} {
		var output bytes.Buffer
		err := run(t.Context(), nil, func(string) string { return dsn }, &output, &output)
		if err == nil || strings.Contains(err.Error(), "secret-password") || output.Len() != 0 {
			t.Fatalf("error=%v output=%s", err, &output)
		}
	}
}

func TestHelpDoesNotReadSecrets(t *testing.T) {
	var output bytes.Buffer
	err := run(t.Context(), []string{"--help"}, func(string) string { t.Fatal("read environment for help"); return "" }, &output, &output)
	if err != nil || !strings.Contains(output.String(), "dsn-env") {
		t.Fatalf("help error=%v output=%s", err, &output)
	}
}

func TestRuntimeErrorsRedactCredentialsAndKeepCause(t *testing.T) {
	cause := errors.Join(context.DeadlineExceeded, errors.New("postgres://user:secret@host/db failed with password secret"))
	err := &redactedError{cause: cause, dsn: "postgres://user:secret@host/db", password: "secret"}
	if strings.Contains(err.Error(), "secret") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("redaction=%v", err)
	}
}
