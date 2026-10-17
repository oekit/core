package chain_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oekit/core/chain"
)

func TestChainOrder(t *testing.T) {
	var order []string
	mk := func(name string) chain.Middleware {
		return func(next chain.Handler) chain.Handler {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, name+"-in")
				resp, err := next(ctx, req)
				order = append(order, name+"-out")
				return resp, err
			}
		}
	}
	h := chain.Finalize(func(ctx context.Context, req any) (any, error) {
		order = append(order, "terminal")
		return "ok", nil
	}, mk("a"), mk("b"))
	got, err := h(context.Background(), nil)
	if err != nil || got != "ok" {
		t.Fatalf("h = %v, %v", got, err)
	}
	want := []string{"a-in", "b-in", "terminal", "b-out", "a-out"}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestChainEmpty(t *testing.T) {
	h := chain.Chain()(func(ctx context.Context, req any) (any, error) {
		return req, nil
	})
	resp, err := h(context.Background(), "req")
	if err != nil || resp != "req" {
		t.Fatalf("empty chain should pass through, got %v %v", resp, err)
	}
}

func TestChainShortCircuit(t *testing.T) {
	sentinel := errors.New("stop")
	reached := false
	h := chain.Finalize(func(ctx context.Context, req any) (any, error) {
		reached = true
		return nil, nil
	},
		func(next chain.Handler) chain.Handler {
			return func(ctx context.Context, req any) (any, error) {
				return nil, sentinel
			}
		},
	)
	_, err := h(context.Background(), nil)
	if !errors.Is(err, sentinel) || reached {
		t.Fatalf("err = %v, reached = %v", err, reached)
	}
}

func TestRecoveryConvertsPanic(t *testing.T) {
	var reported chain.PanicError
	h := chain.Finalize(func(ctx context.Context, req any) (any, error) {
		panic("boom")
	}, chain.Recovery(chain.WithPanicReporter(func(_ context.Context, _ any, pv chain.PanicError) {
		reported = pv
	})))
	resp, err := h(context.Background(), "req")
	if resp != nil {
		t.Fatal("resp should be nil after panic")
	}
	var pe *chain.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *PanicError", err)
	}
	if pe.Value != "boom" || !strings.Contains(pe.Stack, "goroutine") {
		t.Fatalf("PanicError = %+v", pe)
	}
	if reported.Value != "boom" {
		t.Fatalf("reporter not invoked: %+v", reported)
	}
	if !strings.Contains(pe.Error(), "boom") {
		t.Fatalf("Error() = %q", pe.Error())
	}
}

func TestRecoveryPassesThrough(t *testing.T) {
	h := chain.Finalize(func(ctx context.Context, req any) (any, error) {
		return "fine", nil
	}, chain.Recovery())
	resp, err := h(context.Background(), nil)
	if err != nil || resp != "fine" {
		t.Fatalf("recovery should not affect normal flow, got %v %v", resp, err)
	}
}

func TestTimeoutExpiry(t *testing.T) {
	h := chain.Finalize(func(ctx context.Context, req any) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, chain.Timeout(30*time.Millisecond))
	_, err := h(context.Background(), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

func TestTimeoutFastPath(t *testing.T) {
	h := chain.Finalize(func(ctx context.Context, req any) (any, error) {
		return "quick", nil
	}, chain.Timeout(time.Second))
	resp, err := h(context.Background(), nil)
	if err != nil || resp != "quick" {
		t.Fatalf("got %v %v", resp, err)
	}
}
