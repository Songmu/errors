package errors_test

import (
	"fmt"
	"io"

	"github.com/Songmu/errors"
)

func ExampleErrorf() {
	err := errors.Errorf("read config: %w", io.EOF)
	fmt.Println(err)
	fmt.Println(errors.Is(err, io.EOF))
	// Output:
	// read config: EOF
	// true
}

func ExampleJoin() {
	err := errors.Join(errors.New("read failed"), errors.New("cleanup failed"))
	fmt.Println(err)
	// Output:
	// read failed
	// cleanup failed
}

func ExampleFormatter() {
	err := fmt.Errorf("start service: %w", errors.New("read failed"))
	// Use %+v instead of %s to include the retained stack trace.
	fmt.Printf("%s\n", errors.Formatter(err))
	// Output:
	// start service: read failed
}
