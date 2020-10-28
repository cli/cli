package shared

import (
	"fmt"
	"time"
)

func PreserveInput(ims *IssueMetadataState, defs Defaults, doPreserve *bool) func() {
	return func() {
		if ims.Body == defs.Body && ims.Title == defs.Title {
			return
		}

		if !*doPreserve {
			return
		}

		dumpPath := fmt.Sprintf(".dump-%x.json", time.Now().UnixNano())

		// TODO try serializing ims as JSON and dumping to a file with random name
		fmt.Println("DUMPIN TO", dumpPath)
	}
}
