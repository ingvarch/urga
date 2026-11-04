//go:build race

package ui

// raceOn says the tests run with the race detector, which drops what
// sync.Pool is given at random: a count of allocations varies from run to run.
const raceOn = true
