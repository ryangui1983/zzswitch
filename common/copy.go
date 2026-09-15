package common

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jinzhu/copier"
)

// Cloner is implemented by types that provide a fast Clone method,
// bypassing reflection-based deep copy.
type Cloner[T any] interface {
	Clone() *T
}

func DeepCopy[T any](src *T) (*T, error) {
	if src == nil {
		return nil, fmt.Errorf("copy source cannot be nil")
	}
	if c, ok := any(src).(Cloner[T]); ok {
		return c.Clone(), nil
	}
	var dst T
	err := copier.CopyWithOption(&dst, src, copier.Option{
		DeepCopy: true, IgnoreEmpty: true,
		Converters: []copier.TypeConverter{{
			SrcType: json.RawMessage{},
			DstType: json.RawMessage{},
			Fn: func(src any) (any, error) {
				// Copy raw JSON in bulk while retaining independent storage for
				// request mutation and retries, instead of reflecting over each byte.
				return json.RawMessage(bytes.Clone(src.(json.RawMessage))), nil
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	return &dst, nil
}
