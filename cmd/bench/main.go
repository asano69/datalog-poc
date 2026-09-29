// Command bench measures the cost of the links2hop rule on synthetic data,
// the way the server would run it: for one card, load only the facts the
// query needs (like two index lookups on card_links), then evaluate the rule
// over that small set. Nothing is materialized for the whole graph.
//
// It generates n cards; each card links to 1-5 other cards (mean 3). A fixed
// share of the links goes to a small set of "hub" cards (like a popular tag
// card), the rest go to uniformly random cards.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
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
	numSamples = 20  // number of random cards used for the queries

	// rules is parsed once and reused, like the server would do. The card
	// being asked about is not part of the rule: own holds its link targets
	// and link holds the other cards pointing to them, so only the result
	// itself is derived (no all-pairs join).
	rules = `Decl own(Target).
Decl link(Source, Target).
links2hop(B) :- own(X), link(B, X).
`
)

// graph stands in for the card_links collection and its two indexes:
// out is the index on source, in is the index on target.
type graph struct {
	out map[int][]int
	in  map[int][]int
}

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

func generate(n int, rng *rand.Rand) (g *graph, links int) {
	g = &graph{out: map[int][]int{}, in: map[int][]int{}}
	seen := map[[2]int]bool{}
	hubs := max(n/100, 1)
	for from := 0; from < n; from++ {
		for k := 1 + rng.Intn(5); k > 0; k-- {
			to := rng.Intn(n)
			if rng.Float64() < hubShare {
				to = rng.Intn(hubs)
			}
			if to == from || seen[[2]int{from, to}] {
				continue
			}
			seen[[2]int{from, to}] = true
			g.out[from] = append(g.out[from], to)
			g.in[to] = append(g.in[to], from)
			links++
		}
	}
	return g, links
}

func card(i int) ast.Constant { return ast.String(fmt.Sprintf("card%d", i)) }

// loadFacts adds only the facts links2hop needs for cardID: its own link
// targets, and every other card that points to one of those targets.
func loadFacts(g *graph, store factstore.FactStoreWithRemove, cardID int) (loaded int) {
	for _, target := range g.out[cardID] {
		if store.Add(ast.NewAtom("own", card(target))) {
			loaded++
		}
		for _, other := range g.in[target] {
			if other != cardID && store.Add(ast.NewAtom("link", card(other), card(target))) {
				loaded++
			}
		}
	}
	return loaded
}

// query answers links2hop(cardID, _) from scratch and returns the number of
// results and the number of facts that had to be loaded.
func query(g *graph, kind string, unit parse.SourceUnit, cardID int) (results, loaded int) {
	store := newStore(kind)
	loaded = loadFacts(g, store, cardID)

	interp := interpreter.New(io.Discard, ".", nil)
	if err := interp.Preload([]parse.SourceUnit{unit}, store, map[ast.PredicateSym]ast.Decl{}); err != nil {
		log.Fatal(err)
	}
	atom, err := interp.ParseQuery("links2hop(_)")
	if err != nil {
		log.Fatal(err)
	}
	facts, err := interp.Query(atom)
	if err != nil {
		log.Fatal(err)
	}
	return len(facts), loaded
}

func main() {
	n := flag.Int("n", 1000, "number of cards")
	storeKind := flag.String("store", "indexed", "fact store: simple or indexed")
	flag.Parse()

	rng := rand.New(rand.NewSource(1))
	g, links := generate(*n, rng)
	fmt.Printf("n=%d store=%s links=%d\n", *n, *storeKind, links)

	unit, err := parse.Unit(strings.NewReader(rules))
	if err != nil {
		log.Fatal(err)
	}

	// The worst case: a card that links to the most popular target.
	busiest := 0
	for target, sources := range g.in {
		if len(sources) > len(g.in[busiest]) {
			busiest = target
		}
	}
	samples := []int{g.in[busiest][0]}
	for i := 0; i < numSamples; i++ {
		samples = append(samples, rng.Intn(*n))
	}

	var times []time.Duration
	totalResults, totalLoaded := 0, 0
	for _, s := range samples {
		start := time.Now()
		results, loaded := query(g, *storeKind, unit, s)
		elapsed := time.Since(start)
		times = append(times, elapsed)
		totalResults += results
		totalLoaded += loaded
		if s == samples[0] {
			fmt.Printf("%-12s time=%-10v results=%d facts loaded=%d\n", "worst case", elapsed, results, loaded)
		}
	}

	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	fmt.Printf("%-12s median=%-10v max=%-10v mean results=%d mean facts loaded=%d\n",
		"per query", times[len(times)/2], times[len(times)-1],
		totalResults/len(samples), totalLoaded/len(samples))
}
