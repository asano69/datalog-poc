// Command bench measures the cost of the links2hop rule on synthetic data.
//
// It generates n cards; each card links to 1-5 other cards (mean 3). A fixed
// share of the links goes to a small set of "hub" cards (like the "fruit"
// card), the rest go to uniformly random cards. It then reports:
//   - the time and heap needed to materialize every rule (Interpreter.Preload)
//   - the time of a single-card query answered from the materialized result
//   - the time of the same query computed on demand, without materializing
//
// Run one process per (n, store) combination so that heap numbers stay clean.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/mangle/ast"
	"github.com/google/mangle/factstore"
	"github.com/google/mangle/interpreter"
	"github.com/google/mangle/parse"
)

const (
	hubShare   = 0.3 // probability that a link targets a hub
	numSamples = 20  // number of cards used for the single-card queries
)

func newStore(kind string) factstore.FactStoreWithRemove {
	switch kind {
	case "simple":
		return factstore.NewSimpleInMemoryStore()
	case "indexed":
		return factstore.NewMultiIndexedInMemoryStore()
	}
	log.Fatalf("unknown -store %q (want simple or indexed)", kind)
	return nil
}

// generate adds n cards' links as triple(Card, "link", Target) facts to store
// and returns the number of links2hop facts the data will produce: a target
// with in-degree d yields d*(d-1) ordered pairs.
func generate(n int, store factstore.FactStoreWithRemove, rng *rand.Rand) (links, pairs int) {
	hubs := max(n/100, 1)
	inDegree := map[int]int{}
	for from := 0; from < n; from++ {
		for k := 1 + rng.Intn(5); k > 0; k-- {
			to := rng.Intn(n)
			if rng.Float64() < hubShare {
				to = rng.Intn(hubs)
			}
			if to == from {
				continue
			}
			if store.Add(ast.NewAtom("triple", card(from), ast.String("link"), card(to))) {
				inDegree[to]++
				links++
			}
		}
	}
	for _, d := range inDegree {
		pairs += d * (d - 1)
	}
	return links, pairs
}

func card(i int) ast.Constant { return ast.String(fmt.Sprintf("card%d", i)) }

func heapMB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / 1e6
}

// timeQueries runs fn for each sample card and prints median/max latency and
// the mean result count.
func timeQueries(label string, samples []int, fn func(cardName string) int) {
	var times []time.Duration
	total := 0
	for _, s := range samples {
		start := time.Now()
		total += fn(fmt.Sprintf("card%d", s))
		times = append(times, time.Since(start))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	fmt.Printf("%-12s median=%-10v max=%-10v mean results=%d\n",
		label, times[len(times)/2], times[len(times)-1], total/len(samples))
}

func main() {
	n := flag.Int("n", 1000, "number of cards")
	storeKind := flag.String("store", "indexed", "fact store: simple (as in cmd/query) or indexed")
	rulesPath := flag.String("rules", "mangle/rules.mg", "path to the Mangle rules file")
	materialize := flag.Bool("materialize", true, "also measure full materialization (slow and memory hungry for large n)")
	maxPairs := flag.Int("maxpairs", 20_000_000, "skip the run if links2hop would exceed this many facts")
	flag.Parse()

	rng := rand.New(rand.NewSource(1))
	store := newStore(*storeKind)
	base := heapMB()
	links, pairs := generate(*n, store, rng)
	fmt.Printf("n=%d store=%s links=%d expected links2hop facts=%d\n", *n, *storeKind, links, pairs)
	if pairs > *maxPairs {
		fmt.Println("skipped: too many links2hop facts")
		return
	}
	fmt.Printf("%-12s heap=%.0f MB\n", "facts only", heapMB()-base)

	samples := make([]int, numSamples)
	for i := range samples {
		samples[i] = rng.Intn(*n)
	}

	// On demand: a per-card rule evaluated over the raw facts. The derived
	// facts go to a throwaway store layered on top of the shared one.
	timeQueries("on demand", samples, func(name string) int {
		src := fmt.Sprintf("Decl triple(S, P, O).\nlinked(B) :- triple(%q, P, X), triple(B, P, X), B != %q.\n", name, name)
		unit, err := parse.Unit(strings.NewReader(src))
		if err != nil {
			log.Fatal(err)
		}
		interp := interpreter.New(io.Discard, ".", nil)
		tee := factstore.NewTeeingStore(store)
		if err := interp.Preload([]parse.SourceUnit{unit}, tee, map[ast.PredicateSym]ast.Decl{}); err != nil {
			log.Fatal(err)
		}
		return countQuery(interp, "linked(_)")
	})

	if !*materialize {
		return
	}

	// Materialized: evaluate all rules once, then answer from the result.
	rulesFile, err := os.Open(*rulesPath)
	if err != nil {
		log.Fatal(err)
	}
	unit, err := parse.Unit(bufio.NewReader(rulesFile))
	rulesFile.Close()
	if err != nil {
		log.Fatal(err)
	}
	interp := interpreter.New(io.Discard, ".", nil)
	start := time.Now()
	if err := interp.Preload([]parse.SourceUnit{unit}, store, map[ast.PredicateSym]ast.Decl{}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%-12s time=%v heap=%.0f MB (total, incl. facts)\n", "materialize", time.Since(start), heapMB()-base)

	timeQueries("materialized", samples, func(name string) int {
		return countQuery(interp, fmt.Sprintf("links2hop(%q, _)", name))
	})
}

func countQuery(interp *interpreter.Interpreter, q string) int {
	atom, err := interp.ParseQuery(q)
	if err != nil {
		log.Fatal(err)
	}
	facts, err := interp.Query(atom)
	if err != nil {
		log.Fatal(err)
	}
	return len(facts)
}
