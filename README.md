errors
=======

[![Test Status](https://github.com/Songmu/errors/actions/workflows/test.yaml/badge.svg?branch=main)][actions]
[![Coverage Status](https://codecov.io/gh/Songmu/errors/branch/main/graph/badge.svg)][codecov]
[![MIT License](https://img.shields.io/github/license/Songmu/errors)][license]
[![PkgGoDev](https://pkg.go.dev/badge/github.com/Songmu/errors)][PkgGoDev]

[actions]: https://github.com/Songmu/errors/actions?workflow=test
[codecov]: https://codecov.io/gh/Songmu/errors
[license]: https://github.com/Songmu/errors/blob/main/LICENSE
[PkgGoDev]: https://pkg.go.dev/github.com/Songmu/errors

Standard-compatible Go errors with stack traces and no external dependencies.

## Synopsis

```go
import (
    "fmt"
    "io"

    "github.com/Songmu/errors"
)

err := errors.Errorf("read config: %w", io.EOF)
fmt.Println(err)            // read config: EOF
fmt.Println(errors.Is(err, io.EOF)) // true
fmt.Printf("%+v\n", err)    // message, error tree, and stack traces
```

## Description

`New`, `Errorf`, and non-nil `Join` capture the calling goroutine's stack.
Messages, `%w` handling, child order, and single/multiple `Unwrap` shapes follow
the standard library. `Is`, `As`, `AsType`, and `Unwrap` delegate to `errors`;
`ErrUnsupported` is the standard sentinel. There are no `Wrap` or `Cause` APIs:
use `Errorf("operation: %w", err)` for context and `Join` for multiple causes.

`Error()`, `%s`, `%v`, and `%q` remain message-only. `%+v` prints the message,
then a `traces:` tree in outer-to-inner order. Each node has a branch path
(`root/0`, `root/1`, etc.); captured frames show the function followed by an
indented `file:line`. Standard wrappers remain visible as untraced nodes.
The tree format is library-specific, not a Go standard.

The root message is complete whenever `Error()` succeeds. Nested library `Join`
nodes show the number of causes instead of repeating their aggregate messages. Other nested
messages are limited to four lines and 256 bytes, with an explicit truncation
marker. This keeps deeply nested aggregates from flooding diagnostic output.

Standard `fmt.Errorf` and `errors.Join` do not propagate detailed formatting.
If either is outermost, use the output-only adapter:

```go
err := fmt.Errorf("start service: %w", errors.New("read failed"))
fmt.Printf("%+v\n", errors.Formatter(err))
```

`Formatter` neither captures a stack nor wraps the error; `Formatter(nil)`
prints `<nil>`. It follows both forms of `Unwrap` and displays all branches,
including repeated references. It does not read other libraries' stack formats
or recover causes hidden with `%v`.

Stacks retain at most 64 program counters and are expanded only for detailed
output. Common caller-frame suffixes are omitted only between a node and its
nearest traced ancestor on the same branch; the capture frame always remains.
Truncated stacks are marked and not used for suffix comparison. Tree traversal
is limited to 64 levels (root included) and 1024 nodes, with explicit omission
markers. This bounds cyclic traversal without requiring comparable errors;
user-defined `Error` and `Unwrap` methods must still terminate. In detailed output,
panics are marked without preventing later siblings from being displayed.
Concurrent formatting does not mutate captured stacks.

A trace describes the goroutine at construction, not every return step or an
external error's original location. A package-level `New` captures initialization:
add context with `Errorf("operation: %w", ErrSentinel)` at the return site.
Use plain `%w` for wrapping; `%+w` can embed the child's detailed output in the
message, just as with `fmt.Errorf`.

`slog`'s built-in default handler and `slog.TextHandler` format error attributes
with `%+v`, so passing a traced error directly can include traces and local source
paths. `slog.JSONHandler` normally uses `Error()` instead. Pass `err.Error()` for
message-only attributes, or `fmt.Sprintf("%+v", errors.Formatter(err))` for explicit
details. There is no logger-specific integration. Stack capture adds cost; the
small API and lack of dependencies are not a claim of standard-library performance.

## Installation

Requires Go 1.27.3 or later.

```console
% go get github.com/Songmu/errors
```

## Development

Run `go test -race ./...` and `go vet ./...`. Benchmarks separate construction
from ordinary and detailed formatting: `go test -run '^$' -bench . -benchmem`.

After changing GitHub Actions dependencies, run `gh actions-lock` and commit
`.github/workflows/actions.lock` with the workflow changes. Install the tool
with `gh extension install github/gh-actions-lock`.

## Author

[Songmu](https://github.com/Songmu)
