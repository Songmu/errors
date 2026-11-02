package errors

import (
	stderrors "errors"
	"fmt"
	"io"
	"testing"
)

var benchmarkError error
var benchmarkText string

func BenchmarkNew(b *testing.B) {
	for _, tc := range []struct {
		name string
		new  func(string) error
	}{{"standard", stderrors.New}, {"traced", New}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkError = tc.new("failed")
			}
		})
	}
}

func BenchmarkErrorf(b *testing.B) {
	for _, tc := range []struct {
		name string
		new  func(string, ...any) error
	}{{"standard", fmt.Errorf}, {"traced", Errorf}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkError = tc.new("read: %w", io.EOF)
			}
		})
	}
}

func BenchmarkJoin(b *testing.B) {
	for _, tc := range []struct {
		name string
		new  func(...error) error
	}{{"standard", stderrors.Join}, {"traced", Join}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkError = tc.new(io.EOF, io.ErrClosedPipe)
			}
		})
	}
}

func BenchmarkFormatting(b *testing.B) {
	err := Errorf("read: %w", Join(New("failed"), io.EOF))
	for _, format := range []string{"%v", "%+v"} {
		b.Run(format, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkText = fmt.Sprintf(format, err)
			}
		})
	}
}
