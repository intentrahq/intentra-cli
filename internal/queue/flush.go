package queue

import (
	"fmt"

	"github.com/atbabers/intentra-cli/internal/api"
	"github.com/atbabers/intentra-cli/internal/debug"
)

// FlushWithJWT sends all queued scans using a JWT access token.
// Returns the number of scans successfully sent.
func FlushWithJWT(accessToken string) int {
	queued, err := DequeueAll()
	if err != nil {
		debug.Warn("failed to read offline queue: %v", err)
		return 0
	}
	if len(queued) == 0 {
		return 0
	}

	debug.Log("Flushing %d queued scan(s)", len(queued))
	sent := 0
	for _, qs := range queued {
		if err := api.SendScanWithJWT(qs.Scan, accessToken); err != nil {
			debug.Warn("failed to flush queued scan %s: %v", qs.Scan.ID, err)
			continue
		}
		Remove(qs.Path)
		sent++
		debug.Log("Flushed queued scan: %s", qs.Scan.ID)
	}

	if sent > 0 {
		fmt.Printf("Synced %d offline scan(s) to intentra.sh\n", sent)
	}
	return sent
}
