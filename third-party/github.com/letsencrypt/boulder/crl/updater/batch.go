package updater

import (
	"context"
	"errors"
	"sync"

	"github.com/letsencrypt/boulder/crl"
	"github.com/letsencrypt/boulder/issuance"
)

// RunOnce causes the crlUpdater to update every shard immediately, then exit.
// It will run as many simultaneous goroutines as the configured maxParallelism.
func (cu *crlUpdater) RunOnce(ctx context.Context) error {
	var wg sync.WaitGroup
	atTime := cu.clk.Now()

	type workItem struct {
		issuerNameID issuance.NameID
		shardIdx     int
	}

	var anyErr bool
	var once sync.Once

	shardWorker := func(in <-chan workItem) {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case work, ok := <-in:
				if !ok {
					return
				}
				err := cu.updateShardWithRetry(ctx, atTime, work.issuerNameID, work.shardIdx, nil)
				if err != nil {
					cu.log.AuditErrf(
						"Generating CRL failed: id=[%s] err=[%s]",
						crl.Id(work.issuerNameID, work.shardIdx, crl.Number(atTime)), err)
					once.Do(func() { anyErr = true })
				}
			}
		}
	}

	inputs := make(chan workItem)

	for range cu.maxParallelism {
		wg.Add(1)
		go shardWorker(inputs)
	}

	for _, issuer := range cu.issuers {
		for i := range cu.numShards {
			select {
			case <-ctx.Done():
				close(inputs)
				wg.Wait()
				return ctx.Err()
			case inputs <- workItem{issuerNameID: issuer.NameID(), shardIdx: i + 1}:
			}
		}
	}
	close(inputs)

	wg.Wait()
	if anyErr {
		return errors.New("one or more errors encountered, see logs")
	}
	return ctx.Err()
}
