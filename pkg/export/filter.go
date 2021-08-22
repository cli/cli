package export

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/itchyny/gojq"
)

func FilterJSON(w io.Writer, input io.Reader, queryStr string) error {
	query, err := gojq.Parse(queryStr)
	if err != nil {
		return err
	}

	jsonData, err := ioutil.ReadAll(input)
	if err != nil {
		return err
	}

	var responseData interface{}
	err = json.Unmarshal(jsonData, &responseData)
	if err != nil {
		return err
	}

	iter := query.Run(responseData)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return err
		}
		if text, e := jsonScalarToString(v); e == nil {
			_, err := fmt.Fprintln(w, text)
			if err != nil {
				return err
			}
		} else {
			var jsonFragment []byte
			jsonFragment, err = json.Marshal(v)
			if err != nil {
				return err
			}
			_, err = w.Write(jsonFragment)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(w, "\n")
			if err != nil {
				return err
			}
		}
	}

	return nil
}
