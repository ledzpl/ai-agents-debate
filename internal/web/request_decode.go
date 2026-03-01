package web

import (
	"encoding/json"
	"errors"
	"io"
)

func decodeStrictJSON(body io.Reader, out any) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("multiple json values are not allowed")
	}
	return nil
}
