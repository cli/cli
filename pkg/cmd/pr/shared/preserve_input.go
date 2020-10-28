package shared

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"
)

// TODO accept output writing thing

func PreserveInput(ims *IssueMetadataState, defs Defaults, doPreserve *bool) func() {
	return func() {
		if ims.Body == defs.Body && ims.Title == defs.Title {
			return
		}

		if !*doPreserve {
			return
		}

		data, err := json.Marshal(ims)
		if err != nil {
			fmt.Printf("failed to save input to file: %s\n", err)
			fmt.Println("would have saved:")
			fmt.Printf("%v\n", ims)
			return
		}

		dumpPath := fmt.Sprintf("/tmp/gh-dump-%x.json", time.Now().UnixNano())

		err = ioutil.WriteFile(dumpPath, data, 0660)
		if err != nil {
			fmt.Printf("failed to save input to file: %s\n", err)
			fmt.Println("would have saved:")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("%s operation failed.input saved to: %s\n", "TODO", dumpPath)
	}
}
