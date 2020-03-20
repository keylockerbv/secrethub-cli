package masker

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/secrethub/secrethub-go/internals/assert"
	"github.com/secrethub/secrethub-go/pkg/randchar"
)

var maskString = "<redacted by SecretHub>"

func TestMatcher(t *testing.T) {
	tests := []struct {
		matchString     string
		input           string
		useReset        bool
		resetIndex      int
		expectedMatches []int
	}{
		{
			matchString:     "test",
			input:           "test",
			expectedMatches: []int{0},
		},
		{
			matchString:     "test",
			input:           "ttest",
			expectedMatches: []int{1},
		},
		{
			matchString:     "test",
			input:           "testtest",
			expectedMatches: []int{0, 4},
		},
		{
			matchString:     "testtest",
			input:           "test",
			expectedMatches: nil,
		},
		{
			matchString:     "foofoobar",
			input:           "foofoofoobar",
			expectedMatches: []int{3},
		},
		{
			matchString:     "test",
			input:           "123 testtest",
			expectedMatches: []int{4, 8},
		},
		{
			matchString:     "test",
			input:           "t est",
			expectedMatches: nil,
		},
		{
			matchString:     "test",
			input:           "tesat",
			expectedMatches: nil,
		},
		{
			matchString:     "test",
			input:           "tesT",
			expectedMatches: nil,
		},
		{
			matchString:     "t",
			input:           "ttattt",
			expectedMatches: []int{0, 1, 3, 4, 5},
		},
		{
			matchString:     "tt",
			input:           "ttattt",
			expectedMatches: []int{0, 3},
		},
		{
			matchString:     "test",
			input:           "test",
			useReset:        true,
			resetIndex:      0,
			expectedMatches: []int{0},
		},
		{
			matchString:     "test",
			input:           "test",
			useReset:        true,
			resetIndex:      1,
			expectedMatches: nil,
		},
		{
			matchString:     "test",
			input:           "testtest",
			useReset:        true,
			resetIndex:      1,
			expectedMatches: []int{4},
		},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("%s in %s", tc.matchString, tc.input)

		t.Run(name, func(t *testing.T) {
			matcher := sequenceMatcher{sequence: []byte(tc.matchString)}
			var matches []int
			for i, b := range []byte(tc.input) {
				if tc.useReset && tc.resetIndex == i {
					matcher.Reset()
				}

				matchedBytes := matcher.Read(b)
				if matchedBytes > 0 {
					matches = append(matches, i-len(tc.matchString)+1)
				}
			}
			assert.Equal(t, matches, tc.expectedMatches)
		})
	}

}

func TestNewMaskedWriter(t *testing.T) {
	timeout10s := time.Second * 10
	timeout1us := time.Microsecond * 1
	timeout0 := time.Second * 0

	randomIn, err := randchar.NewGenerator(true).Generate(10000)
	assert.OK(t, err)

	tests := map[string]struct {
		maskStrings []string
		inputFunc   func(io.Writer)
		timeout     *time.Duration
		expected    string
	}{
		"no_masking": {
			maskStrings: []string{"foo", "bar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("test"))
				assert.OK(t, err)
			},
			expected: "test",
		},
		"single mask": {
			maskStrings: []string{"foo", "bar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("test foo test"))
				assert.OK(t, err)
			},
			expected: "test " + maskString + " test",
		},
		"multiple masks": {
			maskStrings: []string{"foo", "bar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("test foo bar"))
				assert.OK(t, err)
			},
			expected: "test " + maskString + " " + maskString,
		},
		"incomplete mask": {
			maskStrings: []string{"foobar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("test foo"))
				assert.OK(t, err)
			},
			expected: "test foo",
		},
		"mask within a mask": {
			maskStrings: []string{"foo", "bar", "testfoobartestfoo"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("testfoobartestfoo bar foo"))
				assert.OK(t, err)
			},
			expected: maskString + " " + maskString + " " + maskString,
		},
		"across multiple writes": {
			maskStrings: []string{"foo", "bar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("fo"))
				assert.OK(t, err)
				_, err = w.Write([]byte("o bar f"))
				assert.OK(t, err)
				_, err = w.Write([]byte("o"))
				assert.OK(t, err)
			},
			expected: maskString + " " + maskString + " fo",
		},
		"within timeout": {
			maskStrings: []string{"foo", "bar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("fo"))
				assert.OK(t, err)
				time.Sleep(time.Nanosecond * 1)
				_, err = w.Write([]byte("o test"))
				assert.OK(t, err)
			},
			timeout:  &timeout10s,
			expected: maskString + " test",
		},
		"outside timeout": {
			maskStrings: []string{"foo", "bar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("fo"))
				assert.OK(t, err)
				time.Sleep(time.Second * 2)
				_, err = w.Write([]byte("o bar test"))
				assert.OK(t, err)
			},
			timeout:  &timeout1us,
			expected: "foo " + maskString + " test",
		},
		"no timeout": {
			maskStrings: []string{"foo", "bar"},
			inputFunc: func(w io.Writer) {
				_, err := w.Write([]byte("test foo test"))
				assert.OK(t, err)
			},
			timeout:  &timeout0,
			expected: "test " + maskString + " test",
		},
		"long input": {
			maskStrings: []string{},
			inputFunc: func(w io.Writer) {
				for _, c := range randomIn {
					_, err := w.Write([]byte{c})
					assert.OK(t, err)
				}
			},
			expected: string(randomIn),
		},
		"reuse input buffer": {
			maskStrings: []string{},
			inputFunc: func(w io.Writer) {
				tmp := make([]byte, 1)
				for _, c := range randomIn {
					copy(tmp, []byte{c})
					_, err := w.Write(tmp)
					assert.OK(t, err)
				}
			},
			expected: string(randomIn),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			timeout := time.Millisecond * 10
			if tc.timeout != nil {
				timeout = *tc.timeout
			}

			var maskStrings [][]byte
			for _, s := range tc.maskStrings {
				maskStrings = append(maskStrings, []byte(s))
			}

			m := New(maskStrings, maskString, timeout)
			w := m.NewWriter(&buf)

			go m.Run()
			tc.inputFunc(w)

			err := m.Flush()

			assert.OK(t, err)
			assert.Equal(t, buf.String(), tc.expected)
		})
	}
}

func TestNewMaskedWriter_FlushBeforeTimeout(t *testing.T) {
	// There was a bug in MaskedWriter where it was only flushed on a timeout when a secret was found in the middle
	// of write. This test assures this bug is not present by writing a secret in the middle of a Write and
	// checking whether Flush() returns before the timeout of the MaskedWriter.

	var buf bytes.Buffer

	maskStrings := [][]byte{[]byte("foo")}

	m := New(maskStrings, maskString, time.Second*10)
	w := m.NewWriter(&buf)

	go m.Run()
	_, err := w.Write([]byte("teststring foo more text"))
	assert.OK(t, err)

	done := make(chan struct{})
	go func() {
		err := m.Flush()
		assert.OK(t, err)
		done <- struct{}{}
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Error("MaskedWriter was not flushed before timeout")
	case <-done:
	}
}

type errWriter struct {
	err error
}

func (w errWriter) Write(p []byte) (n int, err error) {
	return 0, w.err
}

func TestNewMaskedWriter_WriteError(t *testing.T) {
	expectedErr := fmt.Errorf("test")

	m := New([][]byte{[]byte("a")}, "aa", time.Millisecond)
	w := m.NewWriter(&errWriter{err: expectedErr})

	go m.Run()
	_, err := w.Write([]byte{0x01})
	assert.OK(t, err)

	err = m.Flush()
	assert.Equal(t, err, expectedErr)
}
