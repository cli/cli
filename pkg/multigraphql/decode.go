package multigraphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type graphqlResponse struct {
	Data   map[string]*json.RawMessage
	Errors []struct {
		Message string
	}
}

// Decode parses the GraphQL JSON response
func Decode(r io.Reader, destinations []interface{}) error {
	resp := graphqlResponse{}
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return err
	}

	if len(resp.Errors) > 0 {
		messages := []string{}
		for _, e := range resp.Errors {
			messages = append(messages, e.Message)
		}
		return fmt.Errorf("GraphQL error: %s", strings.Join(messages, "; "))
	}

	for alias, value := range resp.Data {
		if !strings.HasPrefix(alias, "multi_") {
			continue
		}
		i, _ := strconv.Atoi(strings.TrimPrefix(alias, "multi_"))
		dec := json.NewDecoder(bytes.NewReader([]byte(*value)))
		if err := dec.Decode(destinations[i]); err != nil {
			return err
		}
	}

	return nil
}
