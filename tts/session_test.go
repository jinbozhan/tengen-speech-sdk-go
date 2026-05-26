package tts

import (
	"context"
	"testing"
	"time"
)

// TestEstablishCtxNoDeadline 验证：调用方未带 deadline 时，establishCtx 给建连阶段套默认超时。
// WHY：调用方实测全用无 deadline 的 context.Background()，gateway 预热失败关连接后
// waitConfigDone 会永久挂起；建连默认超时是避免永久挂起（timeout=error）的兜底闸门。
func TestEstablishCtxNoDeadline(t *testing.T) {
	const def = 30 * time.Second
	before := time.Now()
	ctx, cancel := establishCtx(context.Background(), def)
	defer cancel()

	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected derived ctx to have a deadline, got none")
	}
	// 期望 deadline ≈ now+def，给点调度余量
	lo := before.Add(def - 2*time.Second)
	hi := before.Add(def + 1*time.Second)
	if dl.Before(lo) || dl.After(hi) {
		t.Fatalf("deadline %v out of expected range [%v, %v]", dl, lo, hi)
	}
}

// TestEstablishCtxRespectsCallerDeadline 验证：调用方已带 deadline 时，
// establishCtx 不覆盖、原样尊重调用方 deadline。
// WHY：调用方若主动传更短/更长 deadline，建连阶段必须服从，而非被默认 30s 篡改。
func TestEstablishCtxRespectsCallerDeadline(t *testing.T) {
	const def = 30 * time.Second
	parent, parentCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer parentCancel()
	wantDL, _ := parent.Deadline()

	ctx, cancel := establishCtx(parent, def)
	defer cancel()

	gotDL, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected ctx to retain caller deadline, got none")
	}
	if !gotDL.Equal(wantDL) {
		t.Fatalf("deadline overridden: got %v, want caller's %v", gotDL, wantDL)
	}
}

// TestEstablishCtxDefaultActuallyFires 验证：无 deadline ctx 下，默认超时确实会触发 Done()。
// 用极小的 def（50ms）避免依赖真实 30s，证明 timeout=error 语义生效。
func TestEstablishCtxDefaultActuallyFires(t *testing.T) {
	const def = 50 * time.Millisecond
	ctx, cancel := establishCtx(context.Background(), def)
	defer cancel()

	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", ctx.Err())
		}
	case <-time.After(def + 500*time.Millisecond):
		t.Fatal("derived ctx did not fire within expected window")
	}
}
