package eval

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type helper interface {
	Helper()
}

type skipper interface {
	helper
	SkipNow()
}

// skipIfNotEvaluating skips the test if "go test" is not being run with "-test.run=TestEval".
// Returns whether the test was skipped.
func skipIfNotEvaluating(t skipper) {
	t.Helper()

	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.run=TestEval") {
			return
		}
	}

	t.SkipNow()
}

// Run an evaluation.
// Behaves similar to [testing.T.Run], except it skips the test if "go test" is not being run with "-test.run=TestEval".
// The evaluation function [f] is passed an [E] to help with scoring, logging, etc.
func Run(t *testing.T, name string, f func(t *testing.T, e *E)) {
	t.Helper()

	t.Run(name, func(t *testing.T) {
		skipIfNotEvaluating(t)

		e := &E{t: t}
		e.ResetTimer()

		f(t, e)
	})
}

type E struct {
	Group string
	start time.Time
	t     *testing.T
}

// ResetTimer zeroes the elapsed eval time.
// Similar to [testing.B.ResetTimer].
func (e *E) ResetTimer() {
	e.start = time.Now()
}

// Score a [Sample] using a [Scorer] and return the [Result].
// This is just a convenience method to make it easier to swap out scorers.
func (e *E) Score(s Sample, scorer Scorer) Result {
	r := scorer(s)
	r.Score.IsValid()
	return r
}

type logLine struct {
	Name     string
	Group    string
	Sample   Sample
	Results  []Result
	Duration time.Duration
}

var evalsFileLock sync.Mutex
var evalsFileOnce sync.Once

// Log a [Sample] and [Result]-s to evals.jsonl.
// This effectively logs the eval name, sample, and results, along with timing information.
// TODO include token information?
func (e *E) Log(s Sample, rs ...Result) {
	e.t.Helper()

	// If E.Group isn't set, split the name and use the first part before the slash as the group
	group := e.Group
	if group == "" {
		parts := strings.Split(e.t.Name(), "/")
		group = strings.TrimPrefix(parts[0], "TestEval")
	}

	l := logLine{
		Name:     e.t.Name(),
		Group:    group,
		Sample:   s,
		Results:  rs,
		Duration: time.Since(e.start),
	}

	e.t.Logf("%+v", l)

	evalsFileLock.Lock()
	defer evalsFileLock.Unlock()

	dir := findProjectRoot(e.t)
	path := path.Join(dir, "evals.jsonl")

	evalsFileOnce.Do(func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			e.t.Fatal(err)
		}
	})

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write(mustJSON(l)); err != nil {
		e.t.Fatal(err)
	}
}

func mustJSON(l logLine) []byte {
	b, err := json.Marshal(l)
	if err != nil {
		panic(err)
	}
	b = append(b, '\n')

	return b
}

func findProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod file")
		}
		dir = parent
	}
}
