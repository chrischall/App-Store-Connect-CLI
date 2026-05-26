package web

import (
	"context"
	"strings"
	"testing"
)

func TestWebReviewIAPsAttachRequiresApp(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--iap-id", "iap-1",
		"--confirm",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		if err := cmd.Exec(context.Background(), nil); err == nil {
			t.Fatal("expected missing --app error")
		}
	})
	if !strings.Contains(stderr, "--app is required") {
		t.Fatalf("expected --app guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsAttachRequiresIAPID(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "app-1",
		"--confirm",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		if err := cmd.Exec(context.Background(), nil); err == nil {
			t.Fatal("expected missing --iap-id error")
		}
	})
	if !strings.Contains(stderr, "--iap-id is required") {
		t.Fatalf("expected --iap-id guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsAttachRequiresConfirm(t *testing.T) {
	cmd := WebReviewIAPsAttachCommand()
	if err := cmd.FlagSet.Parse([]string{
		"--app", "app-1",
		"--iap-id", "iap-1",
	}); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		if err := cmd.Exec(context.Background(), nil); err == nil {
			t.Fatal("expected missing --confirm error")
		}
	})
	if !strings.Contains(stderr, "--confirm is required") {
		t.Fatalf("expected --confirm guidance in stderr, got %q", stderr)
	}
}

func TestWebReviewIAPsGroupCommandReturnsHelpWhenNoSubcommand(t *testing.T) {
	cmd := WebReviewIAPsCommand()
	if cmd.UsageFunc == nil {
		t.Fatal("WebReviewIAPsCommand should set UsageFunc for consistent rendering")
	}
	err := cmd.Exec(context.Background(), nil)
	if err == nil {
		t.Fatal("expected flag.ErrHelp from group Exec with no subcommand")
	}
}
