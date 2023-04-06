package arken

import "github.com/pkg/errors"

func Must[T any](data T, err error) T {
	if err != nil {
		panic(errors.WithStack(err))
	}

	return data
}
