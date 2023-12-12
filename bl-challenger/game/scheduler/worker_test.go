package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/BLASTchain/blast/bl-challenger/game/scheduler/test"
	"github.com/BLASTchain/blast/bl-challenger/game/types"
	"github.com/BLASTchain/blast/bl-e2e/e2eutils/wait"

	"github.com/stretchr/testify/require"
)

func TestWorkerShouldProcessJobsUntilContextDone(t *testing.T) {
	in := make(chan job, 2)
	out := make(chan job, 2)

	ms := &metricSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go progressGames(ctx, in, out, &wg, ms.ThreadActive, ms.ThreadIdle)

	in <- job{
		player: &test.StubGamePlayer{StatusValue: types.GameStatusInProgress},
	}
	waitErr := wait.For(context.Background(), 100*time.Millisecond, func() (bool, error) {
		return ms.activeCalls >= 1, nil
	})
	require.NoError(t, waitErr)
	require.Equal(t, ms.activeCalls, 1)
	require.Equal(t, ms.idleCalls, 1)

	in <- job{
		player: &test.StubGamePlayer{StatusValue: types.GameStatusDefenderWon},
	}
	waitErr = wait.For(context.Background(), 100*time.Millisecond, func() (bool, error) {
		return ms.activeCalls >= 2, nil
	})
	require.NoError(t, waitErr)
	require.Equal(t, ms.activeCalls, 2)
	require.Equal(t, ms.idleCalls, 2)

	result1 := readWithTimeout(t, out)
	result2 := readWithTimeout(t, out)

	require.Equal(t, result1.status, types.GameStatusInProgress)
	require.Equal(t, result2.status, types.GameStatusDefenderWon)

	// Cancel the context which should exit the worker
	cancel()
	wg.Wait()
}

type metricSink struct {
	activeCalls int
	idleCalls   int
}

func (m *metricSink) ThreadActive() {
	m.activeCalls++
}

func (m *metricSink) ThreadIdle() {
	m.idleCalls++
}

func readWithTimeout[T any](t *testing.T, ch <-chan T) T {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		var val T
		t.Fatal("Did not receive event from channel")
		return val // Won't be reached but makes the compiler happy
	case val := <-ch:
		return val
	}
}
