package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
	"google.golang.org/grpc/health"
)

func Test_serverBuilder_initLongRunningCheck(t *testing.T) {
	t.Parallel()
	hs := health.NewServer()
	mockLogger := blog.NewMock()
	sb := &serverBuilder{
		healthSrv:     hs,
		logger:        mockLogger,
		checkInterval: time.Millisecond * 50,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	count := 0
	failEveryThirdCheck := func(context.Context) error {
		count++
		if count%3 == 0 {
			return errors.New("oops")
		}
		return nil
	}
	sb.initLongRunningCheck(ctx, "test", failEveryThirdCheck)
	time.Sleep(time.Millisecond * 110)
	cancel()

	// We expect the following transition timeline:
	//   - ~0ms   1st check passed, NOT_SERVING to SERVING
	//   - ~50ms  2nd check passed, [no transition]
	//   - ~100ms 3rd check failed, SERVING to NOT_SERVING
	serving := mockLogger.GetAllMatching(".*\"NOT_SERVING\" to \"SERVING\"")
	notServing := mockLogger.GetAllMatching((".*\"SERVING\" to \"NOT_SERVING\""))
	test.Assert(t, len(serving) == 1, "expected one serving log line")
	test.Assert(t, len(notServing) == 1, "expected one not serving log line")

	mockLogger.Clear()

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	count = 0
	failEveryOtherCheck := func(context.Context) error {
		count++
		if count%2 == 0 {
			return errors.New("oops")
		}
		return nil
	}
	sb.initLongRunningCheck(ctx, "test", failEveryOtherCheck)
	time.Sleep(time.Millisecond * 110)
	cancel()

	// We expect the following transition timeline:
	//   - ~0ms   1st check passed, NOT_SERVING to SERVING
	//   - ~50ms  2nd check failed, SERVING to NOT_SERVING
	//   - ~100ms 3rd check passed, NOT_SERVING to SERVING
	serving = mockLogger.GetAllMatching(".*\"NOT_SERVING\" to \"SERVING\"")
	notServing = mockLogger.GetAllMatching((".*\"SERVING\" to \"NOT_SERVING\""))
	test.Assert(t, len(serving) == 2, "expected two serving log lines")
	test.Assert(t, len(notServing) == 1, "expected one not serving log line")
}
