package commands

import (
	"fmt"
	"runtime"
	"sync"
)

// parallelResult holds the output of a parallel task.
type parallelResult struct {
	Index  int
	Label  string
	Output string
	Data   interface{}
	Err    error
}

// runParallel executes tasks concurrently with bounded parallelism.
// The task function receives the index and must return (output string, data, error).
// Results are returned in original order.
func runParallel(count int, task func(i int) (string, interface{}, error)) []parallelResult {
	workers := runtime.NumCPU()
	if workers > count {
		workers = count
	}
	if workers < 1 {
		workers = 1
	}

	results := make([]parallelResult, count)
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out, data, err := task(idx)
			results[idx] = parallelResult{Index: idx, Output: out, Data: data, Err: err}
		}(i)
	}

	wg.Wait()
	return results
}

// collectErrors checks results for errors and returns a combined error message.
func collectErrors(results []parallelResult) error {
	var errs []string
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.Label, r.Err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d error(s):\n%s", len(errs), joinLines(errs))
	}
	return nil
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += "  " + l
	}
	return result
}
